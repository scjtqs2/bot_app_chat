package bot

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

const (
	maxImageSize = 1024 * 1024 * 30 // 30MB
)

// 自定义 DNS 配置
var (
	UseCustomDNS = false // 是否使用自定义 DNS，默认为 false
)

func init() {
	// 从环境变量读取配置
	if os.Getenv("USE_CUSTOM_DNS") == "true" || os.Getenv("USE_CUSTOM_DNS") == "1" {
		UseCustomDNS = true
	}
}

// customResolver 使用指定 DNS 服务器的 resolver
var customResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		dnsServers := []string{
			"223.5.5.5:53",   // AliDNS
			"119.29.29.29:53", // DNSPod
		}
		for _, dnsServer := range dnsServers {
			d := net.Dialer{
				Timeout: 5 * time.Second,
			}
			conn, err := d.DialContext(ctx, "udp", dnsServer)
			if err == nil {
				return conn, nil
			}
		}
		// 回退到系统默认
		d := net.Dialer{
			Timeout: 5 * time.Second,
		}
		return d.DialContext(ctx, network, address)
	},
}

// dialContext 自定义拨号函数，使用自定义 DNS
func dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		host = address
		port = "443"
		if network == "tcp" && !strings.Contains(address, ":") {
			port = "80"
		}
	}

	ips, err := customResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		// 如果自定义 DNS 失败，回退到系统默认
		ips, err = net.LookupIP(host)
		if err != nil {
			return nil, err
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no IPs found for %s", host)
	}

	d := net.Dialer{
		Timeout: 30 * time.Second,
	}
	var lastErr error
	for _, ip := range ips {
		addr := net.JoinHostPort(ip.String(), port)
		conn, err := d.DialContext(ctx, network, addr)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// dialTLSContext 自定义 TLS 拨号函数，保留 SNI
func dialTLSContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}

	// 先建立 TCP 连接
	plainConn, err := dialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	// 创建 TLS 连接，保留原始主机名用于 SNI
	tlsConn := tls.Client(plainConn, &tls.Config{
		ServerName:         host, // 关键：设置 SNI
		InsecureSkipVerify: true,
	})

	// 执行 TLS 握手
	handshakeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		plainConn.Close()
		return nil, err
	}

	return tlsConn, nil
}

// NewHTTPTransport 创建 HTTP Transport，根据配置决定是否使用自定义 DNS
func NewHTTPTransport() *http.Transport {
	if UseCustomDNS {
		return &http.Transport{
			ForceAttemptHTTP2:     false,
			MaxConnsPerHost:       0,
			MaxIdleConns:          0,
			MaxIdleConnsPerHost:   999,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			DialContext:           dialContext,
			DialTLSContext:        dialTLSContext,
		}
	}
	// 默认使用系统 DNS，但仍然保留 TLS 配置
	return &http.Transport{
		ForceAttemptHTTP2:     false,
		MaxConnsPerHost:       0,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   999,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
}

// NewHTTPClient 创建使用自定义 DNS 的 HTTP Client
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewHTTPTransport(),
		Timeout:   timeout,
	}
}

var (
	hclientOnce sync.Once
	hclient     *http.Client
)

// getHClient 获取懒加载的 HTTP 客户端
func getHClient() *http.Client {
	hclientOnce.Do(func() {
		hclient = &http.Client{
			Transport: NewHTTPTransport(),
			Timeout:   time.Second * 60,
		}
	})
	return hclient
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

	return getHClient().Do(req)
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
	defer func() { _ = rd.Close() }()
	b, err := io.ReadAll(rd)
	return b, contentType, err
}

// JSON 发送GET请求， 并转换响应为JSON
func (r Request) JSON() (gjson.Result, string, error) {
	rd, contentType, err := r.body()
	if err != nil {
		return gjson.Result{}, contentType, err
	}
	defer func() { _ = rd.Close() }()

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
	defer func() { _ = rd.Close() }()
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
		defer func() { _ = resp.Body.Close() }()
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
		defer func() { _ = file.Close() }()
		_, _ = file.Seek(block.BeginOffset, io.SeekStart)
		writer := bufio.NewWriter(file)
		defer func() { _ = writer.Flush() }()

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
		defer func() { _ = resp.Body.Close() }()
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
