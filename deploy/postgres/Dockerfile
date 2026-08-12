# Postgres with baked-in init scripts.
# Coolify bind-mounts of ./deploy/initdb often miss the host path; baking
# scripts into the image makes first-boot schema+seed reliable.
#
# Build context: ./deploy (see docker-compose.prod.yml).
FROM postgres:16-alpine
COPY initdb/ /docker-entrypoint-initdb.d/
