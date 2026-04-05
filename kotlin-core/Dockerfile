FROM eclipse-temurin:17-jdk-alpine AS build
WORKDIR /app
RUN apk add --no-cache bash
COPY gradle gradle
COPY gradlew gradlew.bat build.gradle.kts settings.gradle.kts gradle.properties ./
COPY src src
RUN chmod +x gradlew \
  && ./gradlew bootJar --no-daemon -x test

FROM eclipse-temurin:17-jre-alpine
WORKDIR /app
RUN apk add --no-cache curl
COPY --from=build /app/build/libs/kotlin-core-0.0.1-SNAPSHOT.jar /app/app.jar
EXPOSE 8081
USER nobody
ENTRYPOINT ["java","-jar","/app/app.jar"]
