package eamqp

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleRPCUsesManualAckForReply(t *testing.T) {
	data, err := os.ReadFile("eamqp.go")
	require.NoError(t, err)
	source := string(data)

	start := strings.Index(source, "func SimpleRPC(")
	require.NotEqual(t, -1, start)
	end := strings.Index(source[start:], "// ReconnectLoop")
	require.NotEqual(t, -1, end)
	simpleRPC := source[start : start+end]

	assert.Contains(t, simpleRPC, `ch.Consume(replyTo, "", false, false, false, false, nil)`)
	assert.NotContains(t, simpleRPC, `ch.Consume(replyTo, "", true, false, false, false, nil)`)
	assert.Contains(t, simpleRPC, `delivery, ok := <-deliveries`)
	assert.Contains(t, simpleRPC, `if err := delivery.Ack(false); err != nil {`)
}
