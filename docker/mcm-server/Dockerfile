FROM eclipse-temurin:21-jre

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl jq \
    && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 25565
ENV EULA=TRUE MCM_DATA_DIR=/data

RUN mkdir -p /data && chmod 777 /data
WORKDIR /data

ENTRYPOINT ["/entrypoint.sh"]
