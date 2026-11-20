package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"testing"
)

// buildClientHello 构造一个带指定 SNI 的最小 TLS 1.3 ClientHello。
func buildClientHello(sni string) []byte {
	var ext bytes.Buffer
	// server_name 扩展（标准格式：uint16 list_len + host_name type + uint16 name_len + name）
	var list bytes.Buffer
	list.Write([]byte{0x00}) // host_name 类型
	var hn bytes.Buffer
	hn.WriteString(sni)
	binary.Write(&list, binary.BigEndian, uint16(hn.Len()))
	list.Write(hn.Bytes())
	var sniBody bytes.Buffer
	binary.Write(&sniBody, binary.BigEndian, uint16(list.Len()))
	sniBody.Write(list.Bytes())

	binary.Write(&ext, binary.BigEndian, uint16(0x0000)) // server_name
	binary.Write(&ext, binary.BigEndian, uint16(sniBody.Len()))
	ext.Write(sniBody.Bytes())
	// 加一个无关键扩展凑数
	var pad bytes.Buffer
	binary.Write(&ext, binary.BigEndian, uint16(0x002b)) // supported_versions
	binary.Write(&ext, binary.BigEndian, uint16(2))
	pad.Write([]byte{0x03, 0x04})
	ext.Write(pad.Bytes())

	var ch bytes.Buffer
	ch.Write([]byte{0x03, 0x03})                       // legacy_version TLS1.2
	ch.Write(make([]byte, 32))                         // random
	ch.Write([]byte{0x00})                             // session_id len
	ch.Write([]byte{0x00, 0x02, 0x13, 0x01})           // cipher_suites: TLS_AES_128_GCM_SHA256
	ch.Write([]byte{0x01, 0x00})                       // compression_methods: null
	binary.Write(&ch, binary.BigEndian, uint16(ext.Len()))
	ch.Write(ext.Bytes())

	var handshake bytes.Buffer
	handshake.Write([]byte{0x01}) // ClientHello
	binary.Write(&handshake, binary.BigEndian, uint32(0)) // 占位 3 字节长度
	b3 := ch.Bytes()
	handshake.Reset()
	handshake.Write([]byte{0x01})
	handshake.Write([]byte{byte(len(b3) >> 16), byte(len(b3) >> 8), byte(len(b3))})
	handshake.Write(b3)

	var rec bytes.Buffer
	rec.Write([]byte{0x16, 0x03, 0x01}) // handshake record
	binary.Write(&rec, binary.BigEndian, uint16(handshake.Len()))
	rec.Write(handshake.Bytes())
	return rec.Bytes()
}

func TestParseSNI(t *testing.T) {
	raw := buildClientHello("www.example.com")
	br := bufio.NewReader(bytes.NewReader(raw))
	sni, err := peekSNI(br)
	if err != nil {
		t.Fatalf("peekSNI error: %v", err)
	}
	if sni != "www.example.com" {
		t.Fatalf("sni = %q, want www.example.com", sni)
	}
	// peek 不消费数据：br 开头仍是 record 头
	head, _ := br.Peek(1)
	if len(head) == 0 || head[0] != 0x16 {
		t.Fatalf("peekSNI consumed data: first byte = %v", head)
	}
}

func TestParseSNIRejectsNonTLS(t *testing.T) {
	br := bufio.NewReader(bytes.NewReader([]byte("GET / HTTP/1.1\r\n")))
	if _, err := peekSNI(br); err == nil {
		t.Fatal("expected error for non-TLS input")
	}
}

func TestParseSNIMissing(t *testing.T) {
	// 无 SNI 扩展的 ClientHello
	var ch bytes.Buffer
	ch.Write([]byte{0x03, 0x03})
	ch.Write(make([]byte, 32))
	ch.Write([]byte{0x00})
	ch.Write([]byte{0x00, 0x02, 0x13, 0x01})
	ch.Write([]byte{0x01, 0x00})
	binary.Write(&ch, binary.BigEndian, uint16(0)) // 无扩展
	var hs bytes.Buffer
	hs.Write([]byte{0x01})
	b3 := ch.Bytes()
	hs.Write([]byte{byte(len(b3) >> 16), byte(len(b3) >> 8), byte(len(b3))})
	hs.Write(b3)
	var rec bytes.Buffer
	rec.Write([]byte{0x16, 0x03, 0x01})
	binary.Write(&rec, binary.BigEndian, uint16(hs.Len()))
	rec.Write(hs.Bytes())

	br := bufio.NewReader(bytes.NewReader(rec.Bytes()))
	if _, err := peekSNI(br); err == nil {
		t.Fatal("expected error when SNI missing")
	}
}
