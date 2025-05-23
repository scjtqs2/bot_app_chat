package bot

import (
	"bufio"
	"compress/gzip"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

func init() {
	// 修正 go1.22之后的 remote error: tls: handshake failure 问题
	os.Setenv("GODEBUG", "tlsrsakex=1")
}

const (
	maxImageSize = 1024 * 1024 * 30 // 30MB
)

var hclient = &http.Client{
	Transport: &http.Transport{
		ForceAttemptHTTP2:   false,
		MaxConnsPerHost:     0,
		MaxIdleConns:        0,
		MaxIdleConnsPerHost: 999,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: false},
	},
	Timeout: time.Second * 60,
}

// ErrOverSize 响应主体过大时返回此错误
var ErrOverSize = errors.New("oversize")

// UserAgent HTTP请求时使用的UA
const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/87.0.4280.88 Safari/537.36 Edg/87.0.664.66"

// Request is a file download request
type Request struct {
	URL    string
	Header map[string]string
	Limit  int64
}

func (r Request) do() (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, r.URL, nil)
	if err != nil {
		return nil, err
	}

	req.Header["User-Agent"] = []string{UserAgent}
	for k, v := range r.Header {
		req.Header.Set(k, v)
	}

	return hclient.Do(req)
}

func (r Request) body() (io.ReadCloser, string, error) {
	resp, err := r.do()
	if err != nil {
		return nil, "", err
	}

	limit := r.Limit // check file size limit
	if limit > 0 && resp.ContentLength > limit {
		_ = resp.Body.Close()
		return nil, "", ErrOverSize
	}

	contentType := resp.Header.Get("Content-Type")

	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		return gzipReadCloser(resp.Body, contentType)
	}

	return resp.Body, contentType, err
}

// Bytes 对给定URL发送Get请求，返回响应主体
func (r Request) Bytes() ([]byte, string, error) {
	rd, contentType, err := r.body()
	if err != nil {
		return nil, contentType, err
	}
	defer rd.Close()
	b, err := io.ReadAll(rd)
	return b, contentType, err
}

// JSON 发送GET请求， 并转换响应为JSON
func (r Request) JSON() (gjson.Result, string, error) {
	rd, contentType, err := r.body()
	if err != nil {
		return gjson.Result{}, contentType, err
	}
	defer rd.Close()

	var sb strings.Builder
	_, err = io.Copy(&sb, rd)
	if err != nil {
		return gjson.Result{}, contentType, err
	}

	return gjson.Parse(sb.String()), contentType, nil
}

func writeToFile(reader io.ReadCloser, path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	_, err = file.ReadFrom(reader)
	return err
}

// WriteToFile 下载到制定目录
func (r Request) WriteToFile(path string) error {
	rd, _, err := r.body()
	if err != nil {
		return err
	}
	defer rd.Close()
	return writeToFile(rd, path)
}

// WriteToFileMultiThreading 多线程下载到制定目录
func (r Request) WriteToFileMultiThreading(path string, thread int) error {
	if thread < 2 {
		return r.WriteToFile(path)
	}

	limit := r.Limit
	type BlockMetaData struct {
		BeginOffset    int64
		EndOffset      int64
		DownloadedSize int64
	}
	var blocks []*BlockMetaData
	var contentLength int64
	errUnsupportedMultiThreading := errors.New("unsupported multi-threading")
	// 初始化分块或直接下载
	initOrDownload := func() error {
		header := make(map[string]string, len(r.Header))
		for k, v := range r.Header { // copy headers
			header[k] = v
		}
		header["range"] = "bytes=0-"
		req := Request{
			URL:    r.URL,
			Header: header,
		}
		resp, err := req.do()
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return errors.New("response status unsuccessful: " + strconv.FormatInt(int64(resp.StatusCode), 10))
		}
		if resp.StatusCode == http.StatusOK {
			if limit > 0 && resp.ContentLength > limit {
				return ErrOverSize
			}
			if err = writeToFile(resp.Body, path); err != nil {
				return err
			}
			return errUnsupportedMultiThreading
		}
		if resp.StatusCode == http.StatusPartialContent {
			contentLength = resp.ContentLength
			if limit > 0 && resp.ContentLength > limit {
				return ErrOverSize
			}
			blockSize := contentLength
			if contentLength > 1024*1024 {
				blockSize = (contentLength / int64(thread)) - 10
			}
			if blockSize == contentLength {
				return writeToFile(resp.Body, path)
			}
			var tmp int64
			for tmp+blockSize < contentLength {
				blocks = append(blocks, &BlockMetaData{
					BeginOffset: tmp,
					EndOffset:   tmp + blockSize - 1,
				})
				tmp += blockSize
			}
			blocks = append(blocks, &BlockMetaData{
				BeginOffset: tmp,
				EndOffset:   contentLength - 1,
			})
			return nil
		}
		return errors.New("unknown status code")
	}
	// 下载分块
	downloadBlock := func(block *BlockMetaData) error {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
		if err != nil {
			return err
		}
		defer file.Close()
		_, _ = file.Seek(block.BeginOffset, io.SeekStart)
		writer := bufio.NewWriter(file)
		defer writer.Flush()

		header := make(map[string]string, len(r.Header))
		for k, v := range r.Header { // copy headers
			header[k] = v
		}
		header["range"] = fmt.Sprintf("bytes=%d-%d", block.BeginOffset, block.EndOffset)
		req := Request{
			URL:    r.URL,
			Header: header,
		}
		resp, err := req.do()
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return errors.New("response status unsuccessful: " + strconv.FormatInt(int64(resp.StatusCode), 10))
		}
		buffer := make([]byte, 1024)
		i, err := resp.Body.Read(buffer)
		for {
			if err != nil && err != io.EOF {
				return err
			}
			i64 := int64(len(buffer[:i]))
			needSize := block.EndOffset + 1 - block.BeginOffset
			if i64 > needSize {
				i64 = needSize
				err = io.EOF
			}
			_, e := writer.Write(buffer[:i64])
			if e != nil {
				return e
			}
			block.BeginOffset += i64
			block.DownloadedSize += i64
			if err == io.EOF || block.BeginOffset > block.EndOffset {
				break
			}
			i, err = resp.Body.Read(buffer)
		}
		return nil
	}

	if err := initOrDownload(); err != nil {
		if err == errUnsupportedMultiThreading {
			return nil
		}
		return err
	}
	wg := sync.WaitGroup{}
	wg.Add(len(blocks))
	var lastErr error
	for i := range blocks {
		go func(b *BlockMetaData) {
			defer wg.Done()
			if err := downloadBlock(b); err != nil {
				lastErr = err
			}
		}(blocks[i])
	}
	wg.Wait()
	return lastErr
}

type gzipCloser struct {
	f io.Closer
	r *gzip.Reader
}

// gzipReadCloser 从 io.ReadCloser 创建 gunzip io.ReadCloser
func gzipReadCloser(reader io.ReadCloser, contentType string) (io.ReadCloser, string, error) {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, contentType, err
	}
	return &gzipCloser{
		f: reader,
		r: gzipReader,
	}, contentType, nil
}

// Read impls io.Reader
func (g *gzipCloser) Read(p []byte) (n int, err error) {
	return g.r.Read(p)
}

// Close impls io.Closer
func (g *gzipCloser) Close() error {
	_ = g.f.Close()
	return g.r.Close()
}
