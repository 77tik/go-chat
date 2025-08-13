include .env
export

.PHONY: up-ollama pull-llama worker-ollama worker-kimi

up-ollama:
	 docker compose -f docker-compose.ollama.yml up -d

pull-llama:
	 docker exec -it ollama ollama pull $(AI_MODEL_OLLAMA)

worker-ollama:
	 AI_PROVIDER=ollama AI_BASE_URL=$(AI_BASE_URL_OLLAMA) AI_MODEL=$(AI_MODEL_OLLAMA) KAFKA_BROKERS=$(KAFKA_BROKERS) AI_JOBS_TOPIC=$(AI_JOBS_TOPIC) AI_RESULTS_TOPIC=$(AI_RESULTS_TOPIC) go run main.go

worker-kimi:
	 AI_PROVIDER=kimi AI_BASE_URL=$(AI_BASE_URL_KIMI) AI_MODEL=$(AI_MODEL_KIMI) AI_API_KEY=$(AI_API_KEY) KAFKA_BROKERS=$(KAFKA_BROKERS) AI_JOBS_TOPIC=$(AI_JOBS_TOPIC) AI_RESULTS_TOPIC=$(AI_RESULTS_TOPIC) go run main.go
