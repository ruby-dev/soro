# Configuration

Configuration precedence remains:

```text
framework defaults
config/application.yaml
config/{SORO_ENV}.yaml
environment variables
```

Application infrastructure can be configured in YAML:

```yaml
log:
  level: info
  format: auto

http:
  port: 8080
  read_header_timeout: 5s
  read_timeout: 15s
  write_timeout: 30s
  idle_timeout: 1m
  shutdown_timeout: 30s
  readiness_timeout: 2s

jobs:
  enabled: true
  default_queue: default
  workers: 10
  shutdown_timeout: 30s

mail:
  transport: smtp
  from: support@example.com
  queue: mailers
  smtp:
    host: smtp.example.com
    port: 587
    username: application
    password: ""
    starttls: true
    implicit_tls: false
    insecure_skip_verify: false
    timeout: 10s

observability:
  enabled: true
  otlp_endpoint: http://collector:4318
  otlp_timeout: 10s
```

Keep secrets such as SMTP passwords in environment variables rather than committed YAML.

Environment variables include:

```text
SORO_LOG_LEVEL
SORO_LOG_FORMAT
SORO_HTTP_READ_HEADER_TIMEOUT
SORO_HTTP_READ_TIMEOUT
SORO_HTTP_WRITE_TIMEOUT
SORO_HTTP_IDLE_TIMEOUT
SORO_HTTP_SHUTDOWN_TIMEOUT
SORO_HTTP_READINESS_TIMEOUT
SORO_JOBS_ENABLED
SORO_JOBS_DEFAULT_QUEUE
SORO_JOBS_WORKERS
SORO_JOBS_SHUTDOWN_TIMEOUT
SORO_MAIL_TRANSPORT
SORO_MAIL_FROM
SORO_MAIL_QUEUE
SMTP_HOST
SMTP_PORT
SMTP_USERNAME
SMTP_PASSWORD
SORO_SMTP_STARTTLS
SORO_SMTP_IMPLICIT_TLS
SORO_SMTP_INSECURE_SKIP_VERIFY
SORO_SMTP_TIMEOUT
SORO_OTEL_ENABLED
OTEL_EXPORTER_OTLP_ENDPOINT
SORO_OTEL_TIMEOUT
```

Durations use Go duration syntax such as `5s` or `1m`. Boolean values use values accepted by `strconv.ParseBool`. Invalid configuration fails application startup. Production requires a database URL and the SMTP transport; console/capture mail is rejected.

Log levels are `debug`, `info`, `warn`, and `error`. Log formats are `text`,
`json`, and `auto`. Auto uses readable structured text in development/test and
structured JSON in production. Soro's default logger redacts common sensitive
attribute keys including passwords, authorization headers, cookies, API keys,
access/refresh tokens, and client secrets. Applications can add sensitive keys
or a custom `ReplaceAttr` hook through `observability.NewLogger`, then install
the logger with `soro.WithLogger`.
