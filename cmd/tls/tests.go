package main

import (
	"bufio"
	"bytes"
	"fmt"
	tls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

var (
	dialTimeout = time.Duration(15) * time.Second
)

func getConnection(hostname string, addr string) (*tls.UConn, error) {
	klw, err := os.OpenFile("./sslkeylogging.log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("os.OpenFile error: %+v", err)
	}
	config := tls.Config{
		ServerName:                  hostname,
		InsecureSkipVerify:          true,
		NextProtos:                  []string{"h2", "http/1.1"},
		DynamicRecordSizingDisabled: true,
		KeyLogWriter:                klw,
	}

	dialConn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("net.DialTimeout error: %+v", err)
	}

	uTlsConn := tls.UClient(dialConn, &config, tls.HelloCustom)

	if err = uTlsConn.ApplyPreset(getSpec(hostname)); err != nil {
		return nil, fmt.Errorf("uTlsConn.ApplyPreset error: %+v", err)
	}

	if err = uTlsConn.Handshake(); err != nil {
		return nil, fmt.Errorf("uTlsConn.Handshake() error: %+v", err)
	}

	return uTlsConn, err
}

func getSpec(hostname string) *tls.ClientHelloSpec {
	clientHelloSpec := &tls.ClientHelloSpec{
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384, // 0x1302
			0x1303,
			0x1301,
			0xc02b,
			0xc02f,
			0xc02c,
			0xc030,
			0xcca9,
			0xcca8,
			0x009e,
			0x009f,
			0xccaa,
			0xc023,
			0xc027,
			0xc009,
			0xc013,
			0xc024,
			0xc028,
			0xc00a,
			0xc014,
			0x0067,
			0x006b,
			0x009c,
			0x009d,
			0x003c,
			0x003d,
			0x002f,
			0x0035,
			0x00ff,
		},
		CompressionMethods: []byte{0x00},
		Extensions: []tls.TLSExtension{
			&tls.SNIExtension{
				ServerName: hostname,
			},
			&tls.SupportedPointsExtension{
				SupportedPoints: []byte{0x00, 0x01, 0x02},
			},
			&tls.SupportedCurvesExtension{ // supported_groups
				Curves: []tls.CurveID{
					tls.CurveX25519,
					tls.CurveSECP256R1,
					0x001e,
					tls.CurveSECP521R1,
					tls.CurveSECP384R1,
					tls.FakeCurveFFDHE2048,
					tls.FakeCurveFFDHE3072,
					tls.FakeCurveFFDHE4096,
					tls.FakeCurveFFDHE6144,
					tls.FakeCurveFFDHE8192,
				},
			},
			&tls.SessionTicketExtension{
				Session:     nil,
				Ticket:      []byte{},
				Initialized: true,
			},
			&tls.ALPNExtension{
				AlpnProtocols: []string{"h2", "http/1.1"},
			},
			&FakeEncryptThenMacExtension{},
			&tls.ExtendedMasterSecretExtension{},
			&tls.SignatureAlgorithmsExtension{
				SupportedSignatureAlgorithms: []tls.SignatureScheme{
					0x0403,
					0x0503,
					0x0603,
					0x0807,
					0x0808,
					0x081a,
					0x081b,
					0x081c,
					0x0809,
					0x080a,
					0x080b,
					0x0804,
					0x0805,
					0x0806,
					0x0401,
					0x0501,
					0x0601,
					0x0303,
					0x0301,
					0x0302,
					0x0402,
					0x0502,
					0x0602,
				},
			},
			&tls.SupportedVersionsExtension{
				Versions: []uint16{
					tls.VersionTLS13,
					tls.VersionTLS12,
				},
			},
			&tls.PSKKeyExchangeModesExtension{
				Modes: []uint8{0x01},
			},
			&tls.KeyShareExtension{
				KeyShares: []tls.KeyShare{
					//{
					//	Group: tls.CurveX25519,
					//	Data:  []byte{0x98, 0xb6, 0x02, 0x04, 0xf9, 0xc2, 0x8d, 0x4a, 0x1a, 0x23, 0x5c, 0x55, 0x7e, 0xf6, 0x20, 0x71, 0x2c, 0x0e, 0x64, 0x23, 0x16, 0x49, 0x80, 0x6e, 0x4b, 0x6d, 0x27, 0xd9, 0x9d, 0xe1, 0x1d, 0x6e},
					//},
					{
						Group: tls.CurveSECP384R1,
						Data:  nil,
					},
				},
			},
		},
		GetSessionID: nil,
	}
	return clientHelloSpec
}

func getJsonBody() string {
	payload := `{"email": "asdlasd"}`
	return payload
}

func makeRequest() {
	hostName := "myhome.proptech.ru"
	addr := "myhome.proptech.ru:443"
	accountId := os.Getenv("ACCOUNT_ID")
	url := fmt.Sprintf("https://myhome.proptech.ru/auth/v2/auth/%s/password", accountId)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(getJsonBody())))
	if err != nil {
		log.Fatalf("http.NewRequest error: %+v", err)
	}

	req.Header.Set("User-Agent", "Google sdkgphone64x8664 | Android 14 | erth | 8.9.2 (8090200) |  | null | 10c99d90-9899-4a25-926f-067b34bc4a7f | null")
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Accept-Encoding", "gzip")

	connection, err := getConnection(hostName, addr)
	if err != nil {
		log.Fatalf("getConnection error: %+v", err)
	}

	resp, err := requestOverTLS(connection, req)
	if err != nil {
		log.Fatalf("requestOverTLS error: %+v", err)
	}
	fmt.Printf("Response: %+v\n", resp)
	fmt.Printf("Client header: %+v\n", req.Header)
	fmt.Println("Server header:" + resp.Header.Get("Server"))
}

func requestOverTLS(conn *tls.UConn, r *http.Request) (*http.Response, error) {
	return httpGetOverConn(conn, conn.ConnectionState().NegotiatedProtocol, r)
}

func httpGetOverConn(conn net.Conn, alpn string, req *http.Request) (*http.Response, error) {
	switch alpn {
	case "h2":
		log.Println("HTTP/2 enabled")
		req.Proto = "HTTP/2.0"
		req.ProtoMajor = 2
		req.ProtoMinor = 0

		tr := http2.Transport{}
		cConn, err := tr.NewClientConn(conn)
		if err != nil {
			return nil, err
		}
		return cConn.RoundTrip(req)
	case "http/1.1", "":
		log.Println("Using HTTP/1.1")
		req.Proto = "HTTP/1.1"
		req.ProtoMajor = 1
		req.ProtoMinor = 1

		err := req.Write(conn)
		if err != nil {
			return nil, err
		}
		return http.ReadResponse(bufio.NewReader(conn), req)
	default:
		return nil, fmt.Errorf("unsupported ALPN: %v", alpn)
	}
}

func main() {
	makeRequest()
}

//
//func main() {
//	// URL to send the POST request to
//
//	// Convert the payload to JSON
//	jsonData, err := json.Marshal(payload)
//	if err != nil {
//		fmt.Println("Error marshalling JSON:", err)
//		return
//	}
//
//	// Create a new request using http.NewRequest
//	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
//	if err != nil {
//		fmt.Println("Error creating request:", err)
//		return
//	}
//
//	// Set headers
//	req.Header.Set("User-Agent", "Google sdkgphone64x8664 | Android 14 | erth | 8.9.2 (8090200) |  | null | 10c99d90-9899-4a25-926f-067b34bc4a7f | null")
//	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
//	req.Header.Set("Connection", "Keep-Alive")
//	req.Header.Set("Accept-Encoding", "gzip")
//
//
//	uTlsConn := tls.UClient(tcpConn, &config, tls.HelloRandomized)
//	client := &http.Client{
//		Timeout: time.Second * 10,
//		Transport: &http.Transport{
//			TLSClientConfig: &config,
//		},
//	}
//
//	// Send the request
//	resp, err := client.Do(req)
//	if err != nil {
//		fmt.Println("Error sending request:", err)
//		return
//	}
//	defer resp.Body.Close()
//
//	fmt.Println("Server header:" + resp.Header.Get("Server"))
//	fmt.Println("Response status:", resp.Status)
//}
