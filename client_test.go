package fluentbit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	semver "github.com/hashicorp/go-version"
	"github.com/ory/dockertest/v3"
)

var baseURL string

var (
	inputs  = [...]string{"cpu"}
	outputs = [...]string{"stdout"}
)

const (
	version = "1.7"
	// flushInterval in seconds.
	flushInterval = 1
)

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

func testMain(m *testing.M) int {
	pool, err := dockertest.NewPool("")
	if err != nil {
		fmt.Printf("could not create docker pool: %v\n", err)
		return 1
	}

	fluentBitContainer, err := setupFluentBitContainer(pool)
	if err != nil {
		fmt.Printf("could not setup fluent bit container: %v\n", err)
		return 1
	}

	defer func() {
		err := pool.Purge(fluentBitContainer)
		if err != nil {
			fmt.Printf("could not cleanup fluentbit container: %v\n", err)
		}
	}()

	baseURL, err = getFluentBitContainerBaseURL(pool, fluentBitContainer)
	if err != nil {
		fmt.Printf("could not get fluent bit container base URL: %v\n", err)
		return 1
	}

	return m.Run()
}

func setupFluentBitContainer(pool *dockertest.Pool) (*dockertest.Resource, error) {
	args := []string{"/fluent-bit/bin/fluent-bit", "-H"}
	for _, input := range inputs {
		args = append(args, "-i", input)
	}
	for _, output := range outputs {
		args = append(args, "-o", output)
	}
	args = append(args, "-f", strconv.Itoa(flushInterval))

	return pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "fluent/fluent-bit",
		Tag:        version,
		Cmd:        args,
	})
}

func getFluentBitContainerBaseURL(pool *dockertest.Pool, container *dockertest.Resource) (string, error) {
	var baseURL string
	err := pool.Retry(func() error {
		hostPort := container.GetHostPort("2020/tcp")
		if hostPort == "" {
			return errors.New("empty fluentbit container host-port for port 2020")
		}

		baseURL = "http://" + hostPort
		ok, err := ping(baseURL)
		if err != nil {
			return fmt.Errorf("could not ping %q: %w", baseURL, err)
		}

		if !ok {
			return fmt.Errorf("%q not ready yet", baseURL)
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	time.Sleep(time.Second * flushInterval) // to be surer the server is ready.

	return baseURL, nil
}

func ping(u string) (bool, error) {
	resp, err := http.DefaultClient.Get(u)
	if err != nil {
		return false, fmt.Errorf("could not do request: %w", err)
	}

	defer resp.Body.Close()

	ok := resp.StatusCode < http.StatusBadRequest
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return false, fmt.Errorf("could not discard response body: %w", err)
	}

	return ok, nil
}

func TestClient_BuildInfo(t *testing.T) {
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    baseURL,
	}

	ctx := context.Background()
	info, err := client.BuildInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}

	{
		constraint, err := semver.NewConstraint(fmt.Sprintf(">= %s", version))
		if err != nil {
			t.Fatal(err)
		}

		got, err := semver.NewSemver(info.FluentBit.Version)
		if err != nil {
			t.Fatal(err)
		}

		if !constraint.Check(got) {
			t.Fatalf("expected version to be %s; got %q", constraint, got)
		}
	}

	if want, got := "Community", info.FluentBit.Edition; want != got {
		t.Fatalf("expected edition to be %q; got %q", want, got)
	}
}

func TestClient_UpTime(t *testing.T) {
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    baseURL,
	}

	ctx := context.Background()
	up, err := client.UpTime(ctx)
	if err != nil {
		t.Fatal(err)
	}

	want := time.Second
	time.Sleep(want)

	if got := time.Second * time.Duration(up.UpTimeSec); !(got >= want) {
		t.Fatalf("expected uptime to be >= %s; got %q", want, got)
	}
}

func TestClient_Metrics(t *testing.T) {
	client := &Client{
		HTTPClient: http.DefaultClient,
		BaseURL:    baseURL,
	}

	ctx := context.Background()
	mm, err := client.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if want, got := len(inputs), len(mm.Input); want != got {
		t.Fatalf("expected inputs len to be %d; got %q", want, got)
	}

	if want, got := len(outputs), len(mm.Output); want != got {
		t.Fatalf("expected outputs len to be %d; got %q", want, got)
	}

	for _, input := range inputs {
		var found bool
		for got := range mm.Input {
			if strings.HasPrefix(string(got), input+".") { // metric names take the format `plugin_name.ID`.
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected input %q; got %+v", input, metricsInputKeys(mm))
		}
	}

	for _, output := range outputs {
		var found bool
		for got := range mm.Output {
			if strings.HasPrefix(string(got), output+".") { // metric names take the format `plugin_name.ID`.
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected output %q; got %+v", output, metricsOutputKeys(mm))
		}
	}
}

func metricsInputKeys(mm Metrics) []string {
	var out []string
	for k := range mm.Input {
		out = append(out, string(k))
	}
	return out
}

func metricsOutputKeys(mm Metrics) []string {
	var out []string
	for k := range mm.Output {
		out = append(out, string(k))
	}
	return out
}
