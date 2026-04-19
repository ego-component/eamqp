package eamqp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicExamplesUseConfigDrivenClient(t *testing.T) {
	files := []string{
		filepath.Join("examples", "producer", "main.go"),
		filepath.Join("examples", "consumer", "main.go"),
		filepath.Join("examples", "pubsub", "main.go"),
		filepath.Join("examples", "connection-pool", "main.go"),
		filepath.Join("examples", "producer-confirm", "main.go"),
		filepath.Join("examples", "batch-producer", "main.go"),
		filepath.Join("examples", "transaction", "main.go"),
		filepath.Join("examples", "workqueue-publisher", "main.go"),
		filepath.Join("examples", "workqueue-worker", "main.go"),
		filepath.Join("examples", "rpc", "main.go"),
		filepath.Join("examples", "qos", "main.go"),
		filepath.Join("examples", "dead-letter", "main.go"),
		filepath.Join("examples", "retry-consumer-listener", "main.go"),
		filepath.Join("examples", "retry-consumer-sender", "main.go"),
		filepath.Join("examples", "pubsub-fanout", "main.go"),
		filepath.Join("examples", "reconnect", "main.go"),
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			source := string(data)

			assert.Contains(t, source, "exampleconfig.LoadClient")
			assert.NotContains(t, source, "eamqp.New(eamqp.Config")
			assert.NotContains(t, source, "func getAddr()")
			assert.NotContains(t, source, "AMQP_ADDR")
		})
	}
}

func TestQoSExampleCountsQueuePassively(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("examples", "qos", "main.go"))
	require.NoError(t, err)
	source := string(data)

	assert.Contains(t, source, "QueueDeclarePassive(name")
	assert.NotContains(t, source, "ch.Get(name, false)")
}

func TestRPCExampleBuildsResponseJSONStructurally(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("examples", "rpc", "main.go"))
	require.NoError(t, err)
	source := string(data)

	assert.Contains(t, source, "json.Marshal")
	assert.NotContains(t, source, `fmt.Sprintf(`+"`"+`{"result": "processed %s"`)
}

func TestLifecycleOptionsDocumentExplicitBoundary(t *testing.T) {
	data, err := os.ReadFile("options.go")
	require.NoError(t, err)
	source := string(data)

	assert.Contains(t, source, "reserved for a future lifecycle supervisor")
	assert.NotContains(t, source, "called after a successful reconnection")
	assert.NotContains(t, source, "called when the connection is lost")
}

func TestProducerAndConsumerExamplesShareTopology(t *testing.T) {
	producerData, err := os.ReadFile(filepath.Join("examples", "producer", "main.go"))
	require.NoError(t, err)
	consumerData, err := os.ReadFile(filepath.Join("examples", "consumer", "main.go"))
	require.NoError(t, err)

	producerSource := string(producerData)
	consumerSource := string(consumerData)

	assert.Contains(t, producerSource, `exchange := "producer.example"`)
	assert.Contains(t, producerSource, `queue := "producer.example.queue"`)
	assert.Contains(t, consumerSource, `exchange := "producer.example"`)
	assert.Contains(t, consumerSource, `queue := "producer.example.queue"`)
	assert.Contains(t, consumerSource, `ch.QueueBind(queue, "test", exchange, false, nil)`)
}

func TestBatchProducerExampleDoesNotClaimHigherThroughput(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("examples", "batch-producer", "main.go"))
	require.NoError(t, err)
	source := string(data)

	assert.NotContains(t, source, "high-throughput")
	assert.NotContains(t, source, "improving throughput")
	assert.Contains(t, source, "confirm")
	assert.Contains(t, source, "This is a reliability comparison, not a throughput benchmark")
}

func TestContextAndNotifyDocsStateOperationalBoundaries(t *testing.T) {
	channelData, err := os.ReadFile("channel.go")
	require.NoError(t, err)
	clientData, err := os.ReadFile("client.go")
	require.NoError(t, err)

	channelSource := string(channelData)
	clientSource := string(clientData)

	assert.Contains(t, channelSource, "Context currently carries trace propagation")
	assert.Contains(t, channelSource, "does not guarantee publish timeout or cancellation")
	assert.Contains(t, channelSource, "The returned channel must be consumed")
	assert.Contains(t, clientSource, "The returned channel must be consumed")
}
