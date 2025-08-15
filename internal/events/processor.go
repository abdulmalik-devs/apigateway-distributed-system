package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Shopify/sarama"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// EventProcessor handles distributed event processing
type EventProcessor struct {
	kafkaProducer sarama.SyncProducer
	kafkaConsumer sarama.Consumer
	rabbitConn    *amqp.Connection
	rabbitChannel *amqp.Channel
	config        *EventConfig
	logger        *zap.Logger
}

// EventConfig holds event processing configuration
type EventConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Provider string `mapstructure:"provider"` // "kafka" or "rabbitmq"
	Kafka    KafkaConfig
	RabbitMQ RabbitMQConfig
}

// KafkaConfig holds Kafka-specific configuration
type KafkaConfig struct {
	Brokers        []string          `mapstructure:"brokers"`
	Topics         map[string]string `mapstructure:"topics"`
	ConsumerGroup  string            `mapstructure:"consumer_group"`
	ProducerConfig ProducerConfig    `mapstructure:"producer_config"`
}

// RabbitMQConfig holds RabbitMQ-specific configuration
type RabbitMQConfig struct {
	URL       string            `mapstructure:"url"`
	Exchanges map[string]string `mapstructure:"exchanges"`
	Queues    map[string]string `mapstructure:"queues"`
}

// ProducerConfig holds producer-specific settings
type ProducerConfig struct {
	Acks        string `mapstructure:"acks"`
	Compression string `mapstructure:"compression"`
	BatchSize   int    `mapstructure:"batch_size"`
	LingerMs    int    `mapstructure:"linger_ms"`
}

// APIEvent represents an API gateway event
type APIEvent struct {
	Timestamp  time.Time         `json:"timestamp"`
	EventType  string            `json:"event_type"`
	UserID     string            `json:"user_id"`
	Service    string            `json:"service"`
	Path       string            `json:"path"`
	Method     string            `json:"method"`
	StatusCode int               `json:"status_code"`
	Latency    time.Duration     `json:"latency"`
	IPAddress  string            `json:"ip_address"`
	UserAgent  string            `json:"user_agent"`
	Metadata   map[string]string `json:"metadata"`
	TraceID    string            `json:"trace_id,omitempty"`
	SpanID     string            `json:"span_id,omitempty"`
}

// NewEventProcessor creates a new event processor
func NewEventProcessor(config *EventConfig, logger *zap.Logger) (*EventProcessor, error) {
	if !config.Enabled {
		return &EventProcessor{config: config, logger: logger}, nil
	}

	ep := &EventProcessor{
		config: config,
		logger: logger,
	}

	switch config.Provider {
	case "kafka":
		if err := ep.initKafka(); err != nil {
			return nil, fmt.Errorf("failed to initialize Kafka: %w", err)
		}
	case "rabbitmq":
		if err := ep.initRabbitMQ(); err != nil {
			return nil, fmt.Errorf("failed to initialize RabbitMQ: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported event provider: %s", config.Provider)
	}

	logger.Info("Event processor initialized", zap.String("provider", config.Provider))
	return ep, nil
}

// initKafka initializes Kafka connections
func (ep *EventProcessor) initKafka() error {
	// Producer config
	producerConfig := sarama.NewConfig()
	producerConfig.Producer.RequiredAcks = sarama.WaitForAll
	producerConfig.Producer.Retry.Max = 3
	producerConfig.Producer.Return.Successes = true

	// Set compression
	switch ep.config.Kafka.ProducerConfig.Compression {
	case "snappy":
		producerConfig.Producer.Compression = sarama.CompressionSnappy
	case "gzip":
		producerConfig.Producer.Compression = sarama.CompressionGZIP
	case "lz4":
		producerConfig.Producer.Compression = sarama.CompressionLZ4
	}

	// Set batch size and linger
	producerConfig.Producer.Flush.Bytes = ep.config.Kafka.ProducerConfig.BatchSize
	producerConfig.Producer.Flush.Frequency = time.Duration(ep.config.Kafka.ProducerConfig.LingerMs) * time.Millisecond

	// Create producer
	producer, err := sarama.NewSyncProducer(ep.config.Kafka.Brokers, producerConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	// Consumer config
	consumerConfig := sarama.NewConfig()
	consumerConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	consumerConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	// Create consumer
	consumer, err := sarama.NewConsumer(ep.config.Kafka.Brokers, consumerConfig)
	if err != nil {
		producer.Close()
		return fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	ep.kafkaProducer = producer
	ep.kafkaConsumer = consumer

	return nil
}

// initRabbitMQ initializes RabbitMQ connections
func (ep *EventProcessor) initRabbitMQ() error {
	// Connect to RabbitMQ
	conn, err := amqp.Dial(ep.config.RabbitMQ.URL)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	// Create channel
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create RabbitMQ channel: %w", err)
	}

	// Declare exchanges
	for name, exchangeType := range ep.config.RabbitMQ.Exchanges {
		err = ch.ExchangeDeclare(
			name,         // name
			exchangeType, // type
			true,         // durable
			false,        // auto-deleted
			false,        // internal
			false,        // no-wait
			nil,          // arguments
		)
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to declare exchange %s: %w", name, err)
		}
	}

	// Declare queues
	for name, routingKey := range ep.config.RabbitMQ.Queues {
		_, err = ch.QueueDeclare(
			name,  // name
			true,  // durable
			false, // delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // arguments
		)
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to declare queue %s: %w", name, err)
		}

		// Bind queue to exchange
		err = ch.QueueBind(
			name,         // queue name
			routingKey,   // routing key
			"api_events", // exchange
			false,
			nil,
		)
		if err != nil {
			conn.Close()
			return fmt.Errorf("failed to bind queue %s: %w", name, err)
		}
	}

	ep.rabbitConn = conn
	ep.rabbitChannel = ch

	return nil
}

// PublishEvent publishes an event to the configured provider
func (ep *EventProcessor) PublishEvent(event *APIEvent) error {
	if !ep.config.Enabled {
		return nil
	}

	switch ep.config.Provider {
	case "kafka":
		return ep.publishToKafka(event)
	case "rabbitmq":
		return ep.publishToRabbitMQ(event)
	default:
		return fmt.Errorf("unsupported event provider: %s", ep.config.Provider)
	}
}

// publishToKafka publishes an event to Kafka
func (ep *EventProcessor) publishToKafka(event *APIEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Determine topic based on event type
	topic := ep.config.Kafka.Topics["api_events"]
	switch event.EventType {
	case "user_event":
		topic = ep.config.Kafka.Topics["user_events"]
	case "audit_log":
		topic = ep.config.Kafka.Topics["audit_logs"]
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(event.UserID),
		Value: sarama.ByteEncoder(data),
		Headers: []sarama.RecordHeader{
			{Key: []byte("event_type"), Value: []byte(event.EventType)},
			{Key: []byte("service"), Value: []byte(event.Service)},
		},
	}

	partition, offset, err := ep.kafkaProducer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to send message to Kafka: %w", err)
	}

	ep.logger.Debug("Event published to Kafka",
		zap.String("topic", topic),
		zap.Int32("partition", partition),
		zap.Int64("offset", offset),
		zap.String("event_type", event.EventType))

	return nil
}

// publishToRabbitMQ publishes an event to RabbitMQ
func (ep *EventProcessor) publishToRabbitMQ(event *APIEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Determine exchange and routing key based on event type
	exchange := ep.config.RabbitMQ.Exchanges["api_events"]
	routingKey := "api.event"

	switch event.EventType {
	case "user_event":
		exchange = ep.config.RabbitMQ.Exchanges["user_events"]
		routingKey = "user.event"
	case "audit_log":
		routingKey = "audit.log"
	}

	err = ep.rabbitChannel.Publish(
		exchange,   // exchange
		routingKey, // routing key
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         data,
			DeliveryMode: amqp.Persistent,
			Headers: amqp.Table{
				"event_type": event.EventType,
				"service":    event.Service,
				"user_id":    event.UserID,
			},
		})

	if err != nil {
		return fmt.Errorf("failed to publish message to RabbitMQ: %w", err)
	}

	ep.logger.Debug("Event published to RabbitMQ",
		zap.String("exchange", exchange),
		zap.String("routing_key", routingKey),
		zap.String("event_type", event.EventType))

	return nil
}

// StartConsumer starts consuming events from the configured provider
func (ep *EventProcessor) StartConsumer(ctx context.Context, handler func(*APIEvent) error) error {
	if !ep.config.Enabled {
		return nil
	}

	switch ep.config.Provider {
	case "kafka":
		return ep.startKafkaConsumer(ctx, handler)
	case "rabbitmq":
		return ep.startRabbitMQConsumer(ctx, handler)
	default:
		return fmt.Errorf("unsupported event provider: %s", ep.config.Provider)
	}
}

// startKafkaConsumer starts consuming from Kafka
func (ep *EventProcessor) startKafkaConsumer(ctx context.Context, handler func(*APIEvent) error) error {
	// Create consumer group
	group, err := sarama.NewConsumerGroupFromString(ep.config.Kafka.Brokers, ep.config.Kafka.ConsumerGroup, nil)
	if err != nil {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				group.Close()
				return
			default:
				topics := []string{ep.config.Kafka.Topics["api_events"]}
				err := group.Consume(ctx, topics, &kafkaConsumerHandler{
					handler: handler,
					logger:  ep.logger,
				})
				if err != nil {
					ep.logger.Error("Error from consumer", zap.Error(err))
				}
			}
		}
	}()

	return nil
}

// startRabbitMQConsumer starts consuming from RabbitMQ
func (ep *EventProcessor) startRabbitMQConsumer(ctx context.Context, handler func(*APIEvent) error) error {
	msgs, err := ep.rabbitChannel.Consume(
		ep.config.RabbitMQ.Queues["audit_logs"], // queue
		"",                                      // consumer
		false,                                   // auto-ack
		false,                                   // exclusive
		false,                                   // no-local
		false,                                   // no-wait
		nil,                                     // args
	)
	if err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-msgs:
				var event APIEvent
				if err := json.Unmarshal(msg.Body, &event); err != nil {
					ep.logger.Error("Failed to unmarshal event", zap.Error(err))
					msg.Nack(false, true)
					continue
				}

				if err := handler(&event); err != nil {
					ep.logger.Error("Failed to handle event", zap.Error(err))
					msg.Nack(false, true)
					continue
				}

				msg.Ack(false)
			}
		}
	}()

	return nil
}

// Close closes all connections
func (ep *EventProcessor) Close() error {
	var errs []error

	if ep.kafkaProducer != nil {
		if err := ep.kafkaProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Kafka producer: %w", err))
		}
	}

	if ep.kafkaConsumer != nil {
		if err := ep.kafkaConsumer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close Kafka consumer: %w", err))
		}
	}

	if ep.rabbitChannel != nil {
		if err := ep.rabbitChannel.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close RabbitMQ channel: %w", err))
		}
	}

	if ep.rabbitConn != nil {
		if err := ep.rabbitConn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close RabbitMQ connection: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing event processor: %v", errs)
	}

	return nil
}

// kafkaConsumerHandler handles Kafka consumer callbacks
type kafkaConsumerHandler struct {
	handler func(*APIEvent) error
	logger  *zap.Logger
}

func (h *kafkaConsumerHandler) Setup(sarama.ConsumerGroupSession) error   { return nil }
func (h *kafkaConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error { return nil }

func (h *kafkaConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		var event APIEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			h.logger.Error("Failed to unmarshal event", zap.Error(err))
			continue
		}

		if err := h.handler(&event); err != nil {
			h.logger.Error("Failed to handle event", zap.Error(err))
			continue
		}

		session.MarkMessage(message, "")
	}

	return nil
}
