package api

import (
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/require"
)

func TestImageLoadResponseRejectsDaemonErrors(t *testing.T) {
	for _, input := range []string{`{"error":"invalid archive"}`, "{\"stream\":\"loading\"}\n{\"errorDetail\":{\"message\":\"disk full\"}}", `{"stream":`} {
		_, err := readImageLoadResponse(types.ImageLoadResponse{Body: io.NopCloser(strings.NewReader(input)), JSON: true})
		require.Error(t, err)
	}
	for _, isJSON := range []bool{true, false} {
		input := "Loaded image: example:latest\n"
		if isJSON {
			input = `{"stream":"Loaded image: example:latest"}`
		}
		body, err := readImageLoadResponse(types.ImageLoadResponse{Body: io.NopCloser(strings.NewReader(input)), JSON: isJSON})
		require.NoError(t, err)
		require.Equal(t, input, string(body))
	}
}
