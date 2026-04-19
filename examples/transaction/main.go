// transaction demonstrates AMQP channel transactions. All publishes and acks
// within a transaction are committed atomically (TxCommit) or rolled back (TxRollback).
//
// Use cases: multi-step workflows where all steps must succeed or all must be rolled back.
// Note: Transactions have significant overhead. Publisher confirms are usually a better
// choice for reliability without full transaction overhead.
//
// Usage:
//
//	go run ./examples/transaction --config=examples/config/local.toml
package main

import (
	"fmt"
	"log"

	"github.com/ego-component/eamqp"
	"github.com/ego-component/eamqp/examples/internal/exampleconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	client, err := exampleconfig.LoadClient(exampleconfig.DefaultComponentKey)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ch, err := client.NewChannel()
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	defer ch.Close()

	const (
		exchange      = "tx.ex"
		accountQueueA = "tx.account.a"
		accountQueueB = "tx.account.b"
	)

	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		log.Fatalf("Failed to declare exchange: %v", err)
	}
	for _, q := range []string{accountQueueA, accountQueueB} {
		if _, err := ch.QueueDeclare(q, true, false, false, false, nil); err != nil {
			log.Fatalf("Failed to declare queue %s: %v", q, err)
		}
		if err := ch.QueueBind(q, q, exchange, false, nil); err != nil {
			log.Fatalf("Failed to bind queue %s: %v", q, err)
		}
	}

	// Purge queues for clean state.
	ch.QueuePurge(accountQueueA, false)
	ch.QueuePurge(accountQueueB, false)

	// --- Transaction 1: commit ---
	if err := ch.Tx(); err != nil {
		log.Fatalf("Failed to start transaction: %v", err)
	}
	txPublish(ch, exchange, accountQueueA, `{"account": "A", "amount": 100, "type": "credit"}`)
	txPublish(ch, exchange, accountQueueB, `{"account": "B", "amount": 100, "type": "debit"}`)
	if err := ch.TxCommit(); err != nil {
		log.Fatalf("Commit 1 failed: %v", err)
	}
	fmt.Println("Transaction 1 committed.")

	// --- Transaction 2: rollback ---
	if err := ch.Tx(); err != nil {
		log.Fatalf("Failed to start transaction: %v", err)
	}
	txPublish(ch, exchange, accountQueueA, `{"account": "A", "amount": 50, "type": "credit"}`)
	txPublish(ch, exchange, accountQueueB, `{"account": "B", "amount": 50, "type": "debit"}`)
	fmt.Println("Rolling back transaction 2...")
	if err := ch.TxRollback(); err != nil {
		log.Fatalf("Rollback failed: %v", err)
	}

	// --- Transaction 3: commit ---
	if err := ch.Tx(); err != nil {
		log.Fatalf("Failed to start transaction: %v", err)
	}
	txPublish(ch, exchange, accountQueueA, `{"account": "A", "amount": 75, "type": "credit"}`)
	txPublish(ch, exchange, accountQueueB, `{"account": "B", "amount": 75, "type": "debit"}`)
	if err := ch.TxCommit(); err != nil {
		log.Fatalf("Commit 3 failed: %v", err)
	}
	fmt.Println("Transaction 3 committed.")

	// Verify counts using synchronous Get.
	fmt.Println("\n=== Queue Status ===")
	countA := countQueue(ch, accountQueueA)
	countB := countQueue(ch, accountQueueB)
	fmt.Printf("Queue A: %d messages\n", countA)
	fmt.Printf("Queue B: %d messages\n", countB)
	fmt.Println("Expected: 2 messages each (Tx1+Tx3, Tx2 rolled back)")
	fmt.Println("Done.")
}

func txPublish(ch *eamqp.Channel, exchange, routingKey, body string) {
	if err := ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         []byte(body),
	}); err != nil {
		log.Printf("Publish in transaction failed: %v", err)
	} else {
		fmt.Printf("  In tx: %s\n", body)
	}
}

func countQueue(ch *eamqp.Channel, name string) int {
	count := 0
	for {
		_, ok, err := ch.Get(name, false)
		if err != nil || !ok {
			break
		}
		count++
	}
	return count
}
