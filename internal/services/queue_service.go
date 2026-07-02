package services

import (
	"encoding/json"
	"fmt"
	"go-avatar-service/internal/domain"
	"log"
	"sync"

	"github.com/rabbitmq/amqp091-go"
)

type QueueService struct {
	Conn    *amqp091.Connection
	Channel *amqp091.Channel
	Queue   string
	mu      sync.RWMutex
}

func NewRabbitMQService(url, queueName string) (*QueueService, error) {
	// Подключение к RabbitMQ
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Создание канала
	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Объявление очереди
	_, err = channel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		channel.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	return &QueueService{
		Conn:    conn,
		Channel: channel,
		Queue:   queueName,
	}, nil
}

// Publish отправляет событие в очередь
func (q *QueueService) Publish(event interface{}) error {
	// Сериализация события в JSON
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Публикация сообщения
	err = q.Channel.Publish(
		"",      // exchange
		q.Queue, // routing key
		false,   // mandatory
		false,   // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	log.Printf("Published event to queue %s", q.Queue)
	return nil
}

// Close закрывает соединение с RabbitMQ
func (q *QueueService) Close() error {
	if q.Channel != nil {
		q.Channel.Close()
	}
	if q.Conn != nil {
		return q.Conn.Close()
	}
	return nil
}

func (q *QueueService) PublishDeleteEvent(event *domain.AvatarDeleteEvent) error {
	// Для RabbitMQ
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return q.Channel.Publish(
		"",               // exchange
		"avatar.deleted", // routing key
		false,            // mandatory
		false,            // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}

func (q *QueueService) IsConnected() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.Conn != nil && q.Conn.IsClosed() == false
}

func (q *QueueService) Consume(handler func([]byte) error) error {
	msgs, err := q.Channel.Consume(
		"avatars-queue", // queue
		"",              // consumer
		false,           // auto-ack
		false,           // exclusive
		false,           // no-local
		false,           // no-wait
		nil,             // args
	)
	if err != nil {
		return err
	}

	for msg := range msgs {
		err := handler(msg.Body)
		if err == nil {
			msg.Ack(false) // Подтверждаем обработку
		} else {
			msg.Nack(false, true) // Отправляем на повторную обработку
		}
	}

	return nil
}
