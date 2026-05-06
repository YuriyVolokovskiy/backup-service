# backup-service

Минимальный Go-сервис для logical backup PostgreSQL через `pg_dump --format=custom` с выгрузкой `.dump` файлов в S3-compatible bucket и cleanup по retention policy.

## Команды

```bash
backup-service serve --config /etc/backup-service/config.yaml
backup-service backup --config /etc/backup-service/config.yaml --target example-app-prod
backup-service cleanup --config /etc/backup-service/config.yaml --target example-app-prod
backup-service list --config /etc/backup-service/config.yaml --target example-app-prod
```

## Конфигурация

Пример находится в `examples/config.yaml`. Секреты и DSN передаются только через env. При старте сервис валидирует наличие env-переменных, включая `database_url_env` каждого target.

Финальный S3 key:

```text
<s3_prefix>/YYYY/MM/DD/<target_id>-YYYYMMDDTHHMMSSZ.dump
```

Upload сначала идет в `<key>.partial`, затем объект копируется в финальное имя, после чего `.partial` удаляется.

## Docker

```bash
docker build -t backup-service .
docker run --rm \
  --env-file .env \
  -v "$PWD/examples/config.yaml:/etc/backup-service/config.yaml:ro" \
  backup-service
```

Образ содержит `backup-service`, `postgresql-client`, CA certificates и timezone data. Восстановление созданного файла выполняется штатным `pg_restore`.
