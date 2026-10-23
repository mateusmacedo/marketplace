# syntax=docker/dockerfile:1

# Imagem do bff: a única borda REST pública do marketplace, com os papéis api
# (padrão) e session-events no mesmo binário; chama os contextos por gRPC e
# serve o OpenAPI de cada um, copiado para a imagem.
#
# Build a partir da RAIZ do repositório — o go.work resolve os módulos do
# workspace, e o kernel vem por versão do proxy Go, então o COPY leva só as apps:
#   docker build -f apps/backend/bff/Dockerfile -t bff:local .
#
# Esta imagem NÃO entra no release Docker do workspace (nx-release.yml filtra
# `tag:type:app,!tag:stack:go`): ela serve ao compose local (infra/local) e aos
# manifestos Kustomize (infra/k8s).

FROM golang:1.26.10-alpine AS build
WORKDIR /src

COPY go.work go.work.sum ./
COPY apps/backend ./apps/backend

RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' \
      -o /out/bff ./apps/backend/bff/cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bff /bff
COPY apps/backend/tenancy/contract/openapi /contracts/openapi/tenancy
ENV OPENAPI_TENANCY_PATH=/contracts/openapi/tenancy/v1/openapi.yaml
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=4s --start-period=5s --retries=3 CMD ["/bff", "healthcheck"]
ENTRYPOINT ["/bff"]
