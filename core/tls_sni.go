package main

import (
	"bufio"
	"encoding/binary"
	"errors"
)

// peekSNI 不消费字节地从客户端 TLS 流中读取第一条 record，解析 ClientHello 的 server_name（SNI）。
// 返回后 br 中的数据保持完整，可交给 tls.Server 继续握手。
func peekSNI(br *bufio.Reader) (string, error) {
	hdr, err := br.Peek(5)
	if err != nil {
		return "", err
	}
	if hdr[0] != 0x16 { // handshake record
		return "", errors.New("not a TLS handshake record")
	}
	recordLen := int(binary.BigEndian.Uint16(hdr[3:5]))
	full, err := br.Peek(5 + recordLen)
	if err != nil {
		return "", err
	}
	return parseClientHelloSNI(full[5 : 5+recordLen])
}

// parseClientHelloSNI 从 ClientHello record body 中提取 SNI。
func parseClientHelloSNI(body []byte) (string, error) {
	if len(body) < 4 || body[0] != 0x01 { // ClientHello
		return "", errors.New("not a ClientHello")
	}
	ch := body[4:]
	// ClientHello: version(2) random(32) session_id(1+n) cipher_suites(2+n)
	//              compression_methods(1+n) extensions(2+n)
	if len(ch) < 34 {
		return "", errors.New("short ClientHello")
	}
	off := 34
	if len(ch) <= off {
		return "", errors.New("bad session_id")
	}
	off += 1 + int(ch[off])
	if len(ch) <= off+1 {
		return "", errors.New("bad cipher_suites")
	}
	csLen := int(binary.BigEndian.Uint16(ch[off:]))
	off += 2 + csLen
	if len(ch) <= off {
		return "", errors.New("bad compression")
	}
	off += 1 + int(ch[off])
	if len(ch) <= off+1 {
		return "", errors.New("bad extensions")
	}
	extLen := int(binary.BigEndian.Uint16(ch[off:]))
	off += 2
	extEnd := off + extLen
	if extEnd > len(ch) {
		extEnd = len(ch) // ClientHello 跨 record：仅解析已读部分
	}
	for off+4 <= extEnd {
		extType := binary.BigEndian.Uint16(ch[off:])
		extDataLen := int(binary.BigEndian.Uint16(ch[off+2:]))
		off += 4
		if extType == 0 { // server_name
			if off+2 <= extEnd {
				p := off + 2
				if p+3 <= extEnd && ch[p] == 0 { // host_name
					nameLen := int(binary.BigEndian.Uint16(ch[p+1:]))
					if p+3+nameLen <= extEnd && nameLen > 0 {
						name := string(ch[p+3 : p+3+nameLen])
						if len(name) < 256 {
							return name, nil
						}
					}
				}
			}
		}
		off += extDataLen
	}
	return "", errors.New("no SNI extension")
}
