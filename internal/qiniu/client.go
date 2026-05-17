package qiniu

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
	"github.com/rela/qiniu-logs/internal/config"
)

type Client struct {
	mac        *auth.Credentials
	bucketMgr  *storage.BucketManager
	cfg        *config.QiniuConfig
}

type FileInfo struct {
	Key      string
	Size     int64
	MimeType string
	PutTime  time.Time
	LogTime  time.Time // resolved logical time; equals PutTime when unresolved
	Hash     string
}

// ListOptions 控制 ListFiles 的过滤与上限。
// From / To 为零值表示不限边界；服务端不支持时间过滤，由 ListFiles 在客户端按 LogTime 过滤。
type ListOptions struct {
	Limit int
	From  time.Time
	To    time.Time
}

// TimeResolver maps an object key + its PutTime to the logical log time.
// A non-nil error means the time could not be determined from the key.
type TimeResolver func(key string, putTime time.Time) (time.Time, error)

type rawEntry struct {
	Key      string
	Size     int64
	MimeType string
	PutTime  time.Time
	Hash     string
}

// selectFiles applies the time window + limit to already-listed entries.
// Precondition: resolve != nil.
// Unresolved time is excluded when a time filter is active, and included
// (LogTime falling back to PutTime) when there is no filter.
func selectFiles(entries []rawEntry, resolve TimeResolver, opts ListOptions) []FileInfo {
	hasFilter := !opts.From.IsZero() || !opts.To.IsZero()
	var out []FileInfo
	for _, e := range entries {
		logTime, rerr := resolve(e.Key, e.PutTime)
		if rerr != nil {
			if hasFilter {
				continue
			}
			logTime = e.PutTime
		} else {
			if !opts.From.IsZero() && logTime.Before(opts.From) {
				continue
			}
			if !opts.To.IsZero() && logTime.After(opts.To) {
				continue
			}
		}
		out = append(out, FileInfo{
			Key:      e.Key,
			Size:     e.Size,
			MimeType: e.MimeType,
			PutTime:  e.PutTime,
			LogTime:  logTime,
			Hash:     e.Hash,
		})
		if opts.Limit > 0 && len(out) >= opts.Limit {
			return out
		}
	}
	return out
}

func NewClient(cfg *config.QiniuConfig) *Client {
	mac := auth.New(cfg.AccessKey, cfg.SecretKey)
	bucketMgr := storage.NewBucketManager(mac, &storage.Config{
		UseHTTPS: cfg.UseHTTPS,
	})

	return &Client{
		mac:       mac,
		bucketMgr: bucketMgr,
		cfg:       cfg,
	}
}

// resolve must be non-nil; it maps each key+PutTime to its logical time.
func (c *Client) ListFiles(ctx context.Context, prefix string, resolve TimeResolver, opts ListOptions) ([]FileInfo, error) {
	hasFilter := !opts.From.IsZero() || !opts.To.IsZero()

	var raw []rawEntry
	var result []FileInfo
	marker := ""
	batchLimit := 100
	if opts.Limit > 0 && opts.Limit < batchLimit && !hasFilter {
		batchLimit = opts.Limit
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		entries, _, nextMarker, hasNext, err := c.bucketMgr.ListFiles(
			c.cfg.Bucket, prefix, "", marker, batchLimit,
		)
		if err != nil {
			return nil, fmt.Errorf("列举文件失败: %w", err)
		}

		for _, entry := range entries {
			raw = append(raw, rawEntry{
				Key:      entry.Key,
				Size:     entry.Fsize,
				MimeType: entry.MimeType,
				PutTime:  time.Unix(0, entry.PutTime*100),
				Hash:     entry.Hash,
			})
		}

		result = selectFiles(raw, resolve, opts)
		if opts.Limit > 0 && len(result) >= opts.Limit {
			break
		}
		if !hasNext {
			break
		}
		marker = nextMarker
	}

	return result, nil
}

// GetPublicURL returns the unsigned `scheme://domain/key` URL.
// For private buckets this alone is not enough to fetch the object;
// it is meant for display/identification. DownloadFile re-signs via
// GetDownloadURL at the moment of fetch.
func (c *Client) GetPublicURL(key string) string {
	scheme := "https"
	if !c.cfg.UseHTTPS {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/%s", scheme, c.cfg.Domain, key)
}

// GetDownloadURL returns a URL that can actually fetch the object:
// signed (1h TTL) for private buckets, bare for public.
func (c *Client) GetDownloadURL(key string) string {
	if c.cfg.Private {
		deadline := time.Now().Add(time.Hour).Unix()
		return storage.MakePrivateURL(c.mac, c.cfg.Domain, key, deadline)
	}
	return c.GetPublicURL(key)
}

func (c *Client) DownloadFile(ctx context.Context, key string, destPath string, progressFn func(downloaded, total int64)) error {
	url := c.GetDownloadURL(key)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	total := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			_, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("写入文件失败: %w", writeErr)
			}
			downloaded += int64(n)
			if progressFn != nil {
				progressFn(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取响应失败: %w", err)
		}
	}

	return nil
}

func FormatSize(size int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	default:
		return fmt.Sprintf("%d B", size)
	}
}
