package antiblock_client

import (
	"bytes"
	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"log"
	"net/http"
)

type AntiblockClient struct {
	Logger *log.Logger

	client tlsclient.HttpClient
}

func NewAntiblockClient() *AntiblockClient {
	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), defaultClientOptions()...)
	if err != nil {
		log.Fatalf("tls_client.NewHttpClient error: %+v", err)
	}
	ac := &AntiblockClient{
		client: client,
		Logger: log.Default(),
	}

	return ac
}

func defaultClientOptions() []tlsclient.HttpClientOption {
	// for wireshark HTTPS decrypt
	//klw, err := os.OpenFile("./sslkeylogging.log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	//if err != nil {
	//	log.Fatalf("failed to open key log file: %v", err)
	//}
	jar := tlsclient.NewCookieJar()
	return []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(60),
		tlsclient.WithClientProfile(profiles.ConfirmedAndroid),
		tlsclient.WithCookieJar(jar),
		//tlsclient.WithTransportOptions(&tlsclient.TransportOptions{KeyLogWriter: klw}),
	}
}

func (ac *AntiblockClient) Do(req *http.Request) (*http.Response, error) {
	// read body from request and close it to avoid leaks
	body, err := req.GetBody()
	if err != nil {
		ac.Logger.Printf("Failed to get body: %v", err)
		return nil, err
	}
	defer body.Close()

	myBytes := make([]byte, req.ContentLength)

	_, err = body.Read(myBytes)
	if err != nil {
		return nil, err
	}

	fReq, err := fhttp.NewRequest(req.Method, req.URL.String(), bytes.NewReader(myBytes))
	if err != nil {
		ac.Logger.Printf("Failed to create request: %v", err)
		return nil, err
	}

	fReq.Header = fhttp.Header{
		"user-agent":   {"Google sdkgphone64x8664 | Android 14 | erth | 8.9.2 (8090200) |  | null | 10c99d90-9899-4a25-926f-067b34bc4a7f | null"},
		"content-type": {"application/json; charset=UTF-8"},
		"connection":   {"Keep-Alive"},
		fhttp.HeaderOrderKey: {
			"user-agent",
			"content-type",
			"content-length",
			"host",
			"connection",
			"accept-encoding",
		},
	}

	fResp, err := ac.client.Do(fReq)
	if err != nil {
		ac.Logger.Printf("Failed to send request: %v", err)
		return nil, err
	}

	resp := customResponseToHttpResponse(fResp)

	return resp, nil
}

func customResponseToHttpResponse(fResp *fhttp.Response) *http.Response {
	return &http.Response{
		Status:        fResp.Status,
		StatusCode:    fResp.StatusCode,
		Proto:         fResp.Proto,
		ProtoMajor:    fResp.ProtoMajor,
		ProtoMinor:    fResp.ProtoMinor,
		Header:        customHeadersToHttpHeaders(fResp.Header),
		Body:          fResp.Body,
		ContentLength: fResp.ContentLength,
	}
}

func customHeadersToHttpHeaders(headers fhttp.Header) http.Header {
	httpHeaders := http.Header{}

	for key, values := range headers {
		httpHeaders[key] = values
	}

	return httpHeaders
}
