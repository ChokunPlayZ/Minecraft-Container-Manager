FROM eclipse-temurin:21-jre

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl jq \
    && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

VOLUME /data
WORKDIR /data

EXPOSE 25565
ENV EULA=TRUE

ENTRYPOINT ["/entrypoint.sh"]
