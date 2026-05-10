#!/bin/bash
# Получаем список всех запущенных серверов
SERVERS=$(docker-compose ps -q go-server)

# Формируем upstream для nginx
UPSTREAM=""
for server in $SERVERS; do
  IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' $server)
  UPSTREAM="$UPSTREAM    server $IP:8080;\n"
done

# Обновляем nginx.conf
sed -i "s/upstream backend {.*}/upstream backend {\n$UPSTREAM}/" nginx.conf

# Перезагружаем nginx
docker-compose exec load-balancer nginx -s reload
