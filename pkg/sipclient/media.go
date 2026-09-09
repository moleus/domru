package sipclient

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// mediaSink advertises receive-only G.711 and discards RTP and RTCP.
// ICE, SRTP, delayed offers and other codecs are deliberately unsupported.
type mediaSink struct {
	rtp, rtcp *net.UDPConn
	once      sync.Once
}

func (m *mediaSink) Close() { m.once.Do(func() { m.rtp.Close(); m.rtcp.Close() }) }
func discard(conn *net.UDPConn) {
	buf := make([]byte, 65535)
	for {
		if _, _, err := conn.ReadFromUDP(buf); err != nil {
			return
		}
	}
}

func answerSDP(offer []byte, ip string, first, last int) ([]byte, *mediaSink, error) {
	text := strings.ReplaceAll(string(offer), "\r\n", "\n")
	if len(offer) > 65536 || !strings.HasPrefix(text, "v=0\n") {
		return nil, nil, fmt.Errorf("unsupported SDP")
	}
	type section struct {
		fields []string
		attrs  []string
	}
	var sections []section
	sessionDirection := "sendrecv"
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "a=ice-") || strings.HasPrefix(line, "a=crypto:") || strings.HasPrefix(line, "a=fingerprint:") || strings.HasPrefix(line, "a=group:BUNDLE") {
			return nil, nil, fmt.Errorf("unsupported SDP transport")
		}
		if strings.HasPrefix(line, "m=") {
			f := strings.Fields(line[2:])
			if len(f) < 4 {
				return nil, nil, fmt.Errorf("invalid SDP media")
			}
			sections = append(sections, section{fields: f})
		} else if len(sections) > 0 {
			sections[len(sections)-1].attrs = append(sections[len(sections)-1].attrs, line)
		} else if line == "a=sendonly" || line == "a=recvonly" || line == "a=inactive" || line == "a=sendrecv" {
			sessionDirection = strings.TrimPrefix(line, "a=")
		}
	}
	selected, payload, codec := -1, "", ""
	for i, s := range sections {
		if s.fields[0] != "audio" || s.fields[1] == "0" || s.fields[2] != "RTP/AVP" {
			continue
		}
		port, err := strconv.Atoi(s.fields[1])
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		direction := sessionDirection
		mappings := map[string]string{"0": "PCMU/8000", "8": "PCMA/8000"}
		for _, a := range s.attrs {
			if a == "a=recvonly" || a == "a=inactive" || a == "a=sendonly" || a == "a=sendrecv" {
				direction = strings.TrimPrefix(a, "a=")
			}
			if strings.HasPrefix(a, "a=rtpmap:") {
				parts := strings.Fields(strings.TrimPrefix(a, "a=rtpmap:"))
				if len(parts) == 2 {
					mappings[parts[0]] = strings.ToUpper(parts[1])
				}
			}
		}
		if direction == "recvonly" || direction == "inactive" {
			continue
		}
		for _, pt := range s.fields[3:] {
			n, err := strconv.Atoi(pt)
			if err != nil || n < 0 || n > 127 {
				continue
			}
			c := strings.TrimSuffix(mappings[pt], "/1")
			if c == "PCMA/8000" || c == "PCMU/8000" {
				selected, payload, codec = i, pt, c
				break
			}
		}
		if selected >= 0 {
			break
		}
	}
	if selected < 0 {
		return nil, nil, fmt.Errorf("SDP has no supported sending G.711 audio")
	}
	var sink *mediaSink
	for port := first; port < last; port += 2 {
		rtp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(ip), Port: port})
		if err != nil {
			continue
		}
		rtcp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(ip), Port: port + 1})
		if err != nil {
			rtp.Close()
			continue
		}
		sink = &mediaSink{rtp: rtp, rtcp: rtcp}
		break
	}
	if sink == nil {
		return nil, nil, fmt.Errorf("no available RTP port pair")
	}
	port := sink.rtp.LocalAddr().(*net.UDPAddr).Port
	var b strings.Builder
	fmt.Fprintf(&b, "v=0\r\no=- %d 1 IN IP4 %s\r\ns=domru\r\nc=IN IP4 %s\r\nt=0 0\r\n", time.Now().UnixNano(), ip, ip)
	for i, s := range sections {
		if i == selected {
			fmt.Fprintf(&b, "m=audio %d RTP/AVP %s\r\na=rtpmap:%s %s\r\na=recvonly\r\na=rtcp:%d\r\n", port, payload, payload, codec, port+1)
		} else {
			fmt.Fprintf(&b, "m=%s 0 %s %s\r\n", s.fields[0], s.fields[2], strings.Join(s.fields[3:], " "))
		}
	}
	go discard(sink.rtp)
	go discard(sink.rtcp)
	return []byte(b.String()), sink, nil
}
