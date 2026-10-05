package registryprobe

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNormalizeURL(t *testing.T) {
	for _, raw := range []string{"https://mirror.example", " https://MIRROR.example/v2/ "} {
		got, err := NormalizeURL(raw)
		if err != nil || got != "https://mirror.example" {
			t.Fatalf("normalize %q: %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{
		"http://mirror.example", "https://user:secret@mirror.example", "https://localhost",
		"https://127.0.0.1", "https://10.0.0.1", "https://[::1]", "https://169.254.169.254",
		"https://100.64.0.1", "https://198.18.0.1", "https://mirror.example/path",
		"https://mirror.example?token=secret", "https://mirror.example#fragment", "https://mirror.example:bad",
	} {
		if _, err := NormalizeURL(raw); err == nil {
			t.Errorf("accepted restricted URL %q", raw)
		}
	}
}

func TestProbeRegistryResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    int
		headers http.Header
		want    string
	}{
		{"v2", 200, http.Header{"Docker-Distribution-Api-Version": {"registry/2.0"}}, "reachable"},
		{"auth", 401, http.Header{"Www-Authenticate": {`Bearer realm="https://auth.example/token"`}}, "auth_required"},
		{"html", 200, http.Header{"Content-Type": {"text/html"}}, "unexpected_response"},
		{"redirect", 302, http.Header{"Location": {"https://127.0.0.1"}}, "unexpected_response"},
		{"limited", 429, nil, "rate_limited"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://mirror.example/v2/" || r.Method != "GET" {
					t.Fatalf("unexpected probe: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Fatal("credentials sent to public probe")
				}
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 6*time.Second {
					t.Fatal("probe must have a bounded deadline")
				}
				return &http.Response{StatusCode: tc.code, Header: tc.headers, Body: io.NopCloser(strings.NewReader("ignored"))}, nil
			})}
			result := check(context.Background(), "https://mirror.example", client)
			if result.Status != tc.want || result.HTTPStatus != tc.code || result.CheckedAt == "" {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestProbeTimeoutAndCancellation(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if result := check(ctx, "https://mirror.example", client); result.Status != "timeout" {
		t.Fatalf("unexpected timeout result: %+v", result)
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if result := check(ctx, "https://mirror.example", client); result.Status != "cancelled" {
		t.Fatalf("unexpected cancel result: %+v", result)
	}
}

func TestProbeNetworkLimitationsAreNotRegistryOutages(t *testing.T) {
	for _, tc := range []struct {
		name      string
		addresses []net.IPAddr
		err       error
		want      string
	}{
		{"fake-ip", []net.IPAddr{{IP: net.ParseIP("198.18.0.7")}}, nil, "network_restricted"},
		{"private", []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil, "network_restricted"},
		{"mixed", []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil, "network_restricted"},
		{"dns", nil, &net.DNSError{Err: "test DNS failure", Name: "mirror.example", IsNotFound: true}, "dns_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newHTTPClient(func(context.Context, string) ([]net.IPAddr, error) {
				return tc.addresses, tc.err
			}, func(context.Context, string, string) (net.Conn, error) {
				t.Fatal("network-limited probe must not dial any address")
				return nil, errors.New("unexpected dial")
			})
			result := check(context.Background(), "https://mirror.example", client)
			if result.Status != tc.want || result.HTTPStatus != 0 {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
}

func TestTransportRejectsRestrictedDNSAndPinsPublicAddress(t *testing.T) {
	for _, tc := range []struct {
		name      string
		addresses []net.IPAddr
		wantDial  bool
	}{
		{"private", []net.IPAddr{{IP: net.ParseIP("192.168.1.2")}}, false},
		{"mixed", []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, false},
		{"public", []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialed := ""
			client := newHTTPClient(func(context.Context, string) ([]net.IPAddr, error) { return tc.addresses, nil },
				func(_ context.Context, _ string, address string) (net.Conn, error) {
					dialed = address
					return nil, errors.New("test dial")
				})
			transport := client.Transport.(*http.Transport)
			if transport.Proxy != nil {
				t.Fatal("probe must not inherit environment proxies")
			}
			_, err := transport.DialContext(context.Background(), "tcp", "mirror.example:443")
			if err == nil {
				t.Fatal("test must not open a connection")
			}
			if (dialed != "") != tc.wantDial {
				t.Fatalf("unexpected dial: %q", dialed)
			}
			if tc.wantDial && dialed != "8.8.8.8:443" {
				t.Fatalf("DNS address was not pinned: %q", dialed)
			}
			if err := client.CheckRedirect(&http.Request{}, nil); !errors.Is(err, http.ErrUseLastResponse) {
				t.Fatal("redirects must not be followed")
			}
		})
	}
}
