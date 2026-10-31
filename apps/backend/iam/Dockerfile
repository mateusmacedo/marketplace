# syntax=docker/dockerfile:1

# Imagem do contexto iam: um binário, dois papéis por `--role` (api gRPC e relay).
#
# Build a partir da RAIZ do repositório — o go.work resolve os módulos do
# workspace, e o kernel vem por versão do proxy Go, então o COPY leva só as apps:
#   docker build -f apps/backend/iam/Dockerfile -t iam:local .
#
# Esta imagem NÃO entra no release Docker do workspace (nx-release.yml filtra
# `tag:type:app,!tag:stack:go`, ADR-041): ela serve ao compose local
# (infra/local) e aos manifestos Kustomize (infra/k8s).

FROM golang:1.26.10-alpine AS build
WORKDIR /src

COPY go.work go.work.sum ./
COPY apps/backend ./apps/backend

RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' \
      -o /out/iam ./apps/backend/iam/cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/iam /iam
EXPOSE 9090
ENTRYPOINT ["/iam"]
CMD ["--role", "api"]
