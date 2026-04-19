package eamqp

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type producerMetricClient struct {
	metrics *producerMetrics
}

func (c producerMetricClient) GetLogger() Logger {
	return nil
}

func (c producerMetricClient) GetMetrics() MetricsCollector {
	return c.metrics
}

type producerMetrics struct {
	NoOpMetrics
	confirmed int
	nacked    int
}

func (m *producerMetrics) RecordMessageConfirmed() {
	m.confirmed++
}

func (m *producerMetrics) RecordMessageNacked() {
	m.nacked++
}

func TestProducerRecordConfirmationRecordsConfirmedMetric(t *testing.T) {
	metrics := &producerMetrics{}
	producer := &Producer{
		channel: &Channel{client: producerMetricClient{metrics: metrics}},
	}

	err := producer.recordConfirmation(true, 1)

	require.NoError(t, err)
	assert.Equal(t, 1, metrics.confirmed)
	assert.Equal(t, 0, metrics.nacked)
}

func TestProducerRecordConfirmationRecordsNackedMetric(t *testing.T) {
	metrics := &producerMetrics{}
	producer := &Producer{
		channel: &Channel{client: producerMetricClient{metrics: metrics}},
	}

	err := producer.recordConfirmation(false, 1)

	require.Error(t, err)
	assert.Equal(t, 0, metrics.confirmed)
	assert.Equal(t, 1, metrics.nacked)
}

func TestNewProducerRejectsNilChannel(t *testing.T) {
	producer, err := NewProducer(nil)

	require.Error(t, err)
	assert.Nil(t, producer)
	assert.Contains(t, err.Error(), "channel is nil")
}

func TestProducerCloseRejectsNilChannel(t *testing.T) {
	producer := &Producer{}

	err := producer.Close()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel is nil")
}

func TestProducerSerializesPublishPaths(t *testing.T) {
	data, err := os.ReadFile("producer.go")
	require.NoError(t, err)
	source := string(data)

	publishWithOptions := source[strings.Index(source, "func (p *Producer) PublishWithOptions"):]
	publishWithOptions = publishWithOptions[:strings.Index(publishWithOptions, "func (p *Producer) PublishAsync")]
	assert.Contains(t, publishWithOptions, "p.mu.Lock()")
	assert.Contains(t, publishWithOptions, "defer p.mu.Unlock()")

	publishWithContextOptions := source[strings.Index(source, "func (p *Producer) PublishWithContextOptions"):]
	publishWithContextOptions = publishWithContextOptions[:strings.Index(publishWithContextOptions, "func (p *Producer) waitForDeferredConfirm")]
	assert.Contains(t, publishWithContextOptions, "p.mu.Lock()")
	assert.Contains(t, publishWithContextOptions, "defer p.mu.Unlock()")
}

func TestProducerConfirmAsyncCloseIntegration(t *testing.T) {
	if os.Getenv("EAMQP_INTEGRATION") != "1" {
		t.Skip("set EAMQP_INTEGRATION=1 to verify producer confirm close behavior")
	}

	client, err := New(Config{
		Addr:       "amqp://guest:guest@127.0.0.1:5672/",
		ClientName: "producer-confirm-close-test",
	})
	require.NoError(t, err)

	ch, err := client.NewChannel()
	require.NoError(t, err)

	suffix := time.Now().UnixNano()
	exchange := fmt.Sprintf("producer-confirm-close.%d", suffix)
	queueName := fmt.Sprintf("producer-confirm-close.%d", suffix)
	routingKey := "confirm.close"

	require.NoError(t, ch.ExchangeDeclare(exchange, "direct", false, true, false, false, nil))
	_, err = ch.QueueDeclare(queueName, false, true, false, false, nil)
	require.NoError(t, err)
	require.NoError(t, ch.QueueBind(queueName, routingKey, exchange, false, nil))

	producer, err := NewProducer(ch, WithConfirm(time.Second))
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		err := producer.Publish(exchange, routingKey, amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(fmt.Sprintf("sync-%d", i)),
		})
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		confirmation, err := producer.PublishAsync(exchange, routingKey, amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(fmt.Sprintf("async-%d", i)),
		})
		require.NoError(t, err)
		require.NotNil(t, confirmation)

		select {
		case <-confirmation.Done():
			require.True(t, confirmation.Acked())
		case <-time.After(2 * time.Second):
			t.Fatalf("async confirmation %d timed out", i)
		}
	}

	closed := make(chan error, 1)
	go func() {
		closed <- producer.Close()
	}()

	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		if conn := client.RawConnection(); conn != nil {
			_ = conn.Close()
		}
		t.Fatal("producer close timed out after deferred confirms")
	}

	require.NoError(t, client.Close())
}
