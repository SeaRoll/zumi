# Zumi

An open-source opinionated Go framework for building APIs.
Zumi is designed to be a simple Go framework that provides tools that are usually
required in production environments, such as routing, database access, caching, and message queues.

## Features

- **Config**: YAML Configuration management with support for environment variables and default values similar to Spring Boot.
- **Server**: A simple HTTP server with routing and middleware support. Also supports OpenAPI generation through `go:generate`.
- **Database**: A database abstraction layer using `pgx` for PostgreSQL. Pagination support is provided through `SelectRowsPageable`.
- **Queue**: A message queue implementation using NATS for pub/sub messaging.
- **Cache**: A caching layer using `valkey` for fast key-value storage, with optional support for sentinel & pubsub messaging.
- **Resilience**: Built-in support for retries and circuit breakers using `failsafe-go`.

## Installation

```sh
go get github.com/SeaRoll/zumi
```

## Usage

Take a look at the [examples](examples) directory for usage examples.

## Future Plans

- Support more message queue implementations such as Kafka.
- Add support for tracing and monitoring through OpenTelemetry/Prometheus (maybe using chi?).

## Contributing

Contributions are welcome! Please open an issue or submit a pull request if you have suggestions or improvements.

## License

This project is licensed under the MIT License.
