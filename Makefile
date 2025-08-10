.PHONY :checkRedis

checkRedis:
	docker exec -it yoyichat-redis /bin/sh
