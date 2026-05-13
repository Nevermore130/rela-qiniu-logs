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
	Hash     string
}

// ListOptions 控制 ListFiles 的过滤与上限。
// From / To 为零值表示不限边界；服务端不支持时间过滤，由 ListFiles 在客户端按 PutTime 过滤。
type ListOptions struct {
	Limit int
	From  time.Time
	To    time.Time
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

func (c *Client) ListFiles(ctx context.Context, userID string, opts ListOptions) ([]FileInfo, error) {
	prefix := userID
	if c.cfg.PathPrefix != "" {
		prefix = fmt.Sprintf("%s/%s", c.cfg.PathPrefix, userID)
	}

	hasTimeFilter := !opts.From.IsZero() || !opts.To.IsZero()

	var files []FileInfo
	marker := ""
	batchLimit := 100
	if opts.Limit > 0 && opts.Limit < batchLimit && !hasTimeFilter {
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
			putTime := time.Unix(0, entry.PutTime*100)
			if !opts.From.IsZero() && putTime.Before(opts.From) {
				continue
			}
			if !opts.To.IsZero() && putTime.After(opts.To) {
				continue
			}
			files = append(files, FileInfo{
				Key:      entry.Key,
				Size:     entry.Fsize,
				MimeType: entry.MimeType,
				PutTime:  putTime,
				Hash:     entry.Hash,
			})
			if opts.Limit > 0 && len(files) >= opts.Limit {
				return files, nil
			}
		}

		if !hasNext {
			break
		}
		marker = nextMarker
	}

	return files, nil
}

func (c *Client) GetDownloadURL(key string) string {
	scheme := "https"
	if !c.cfg.UseHTTPS {
		scheme = "http"
	}

	publicURL := fmt.Sprintf("%s://%s/%s", scheme, c.cfg.Domain, key)

	if c.cfg.Private {
		deadline := time.Now().Add(time.Hour).Unix()
		return storage.MakePrivateURL(c.mac, c.cfg.Domain, key, deadline)
	}

	return publicURL
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
