![Redgres self-hosted PostgreSQL and Redis control plane](web/public/assets/redgres-logo-light.png)

**One secure control plane for PostgreSQL and Redis.**  
Redgres is a Self-hosted database administration for a single PostgreSQL cluster and Redis instance.

![Redgres CI status](https://img.shields.io/github/actions/workflow/status/SSujitX/redgres/ci.yml?branch=master&style=flat-square&label=CI)![GitHub stars](https://img.shields.io/github/stars/SSujitX/redgres?style=flat-square&logo=github)![Project status: development partial](https://img.shields.io/badge/status-development--partial-f59e0b?style=flat-square)![Self-hosted software](https://img.shields.io/badge/deployment-self--hosted-2563eb?style=flat-square)[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square)](LICENSE)

![Go 1.27.0](https://img.shields.io/badge/Go-1.27.0-00ADD8?style=for-the-badge&logo=go&logoColor=white)![React 19.2.8](https://img.shields.io/badge/React-19.2.8-20232A?style=for-the-badge&logo=react&logoColor=61DAFB)![TypeScript 7.0.2](https://img.shields.io/badge/TypeScript-7.0.2-3178C6?style=for-the-badge&logo=typescript&logoColor=white)![PostgreSQL 17 and 18 compatibility targets](https://img.shields.io/badge/PostgreSQL-17%20%7C%2018-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)![Redis 8.2 and 8.8 compatibility targets](https://img.shields.io/badge/Redis-8.2%20%7C%208.8-DC382D?style=for-the-badge&logo=redis&logoColor=white)

Redgres is a self-hosted Go and React administration console for securely managing one PostgreSQL cluster and one Redis instance. It combines PostgreSQL project database provisioning, Redis ACL user management, credential-safe workflows, audit history, health monitoring, and protected links to expert tools in one operator-focused interface.

> [!WARNING]
> Redgres is under active development and is **not production accepted**. Do not retire an existing database console until the migration, recovery, compatibility, staging, and production gates are complete. See the evidence-backed [implementation matrix](docs/TRACEABILITY.md) and [acceptance checklist](docs/ACCEPTANCE_CHECKLIST.md).
