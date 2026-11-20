package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
)

// maxDecodedSize 限制解压后的体积，防止解压炸弹。
const maxDecodedSize = 4 << 20

// readBodyCapped 读取最多 max+1 字节；超出则截断并标记。
func readBodyCapped(r io.Reader, max int) (data []byte, truncated bool, err error) {
	raw, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil {
		return nil, false, err
	}
	if len(raw) > max {
		return raw[:max], true, nil
	}
	return raw, false, nil
}

// decodeBody 按 Content-Encoding 解压（gzip/deflate/br），失败时原样返回。
func decodeBody(data []byte, encoding string) []byte {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return data
		}
		defer zr.Close()
		out, err := io.ReadAll(io.LimitReader(zr, maxDecodedSize))
		if err != nil {
			return data
		}
		return out
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			// 部分实现用的是裸 deflate（无 zlib 头）
			fr := flate.NewReader(bytes.NewReader(data))
			out, err2 := io.ReadAll(io.LimitReader(fr, maxDecodedSize))
			fr.Close()
			if err2 != nil {
				return data
			}
			return out
		}
		defer zr.Close()
		out, err := io.ReadAll(io.LimitReader(zr, maxDecodedSize))
		if err != nil {
			return data
		}
		return out
	case "br":
		br := brotli.NewReader(bytes.NewReader(data))
		out, err := io.ReadAll(io.LimitReader(br, maxDecodedSize))
		if err != nil {
			return data
		}
		return out
	default:
		return data
	}
}
