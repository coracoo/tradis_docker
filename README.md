# TRADIS

面向 NAS 的 Docker 管理面板，通过浏览器管理容器、Compose 项目和本地资源。

## 功能

- 容器启动、停止、日志查看和终端。
- Compose 部署、配置编辑、历史记录与 Git 导入。
- 镜像、卷、网络、端口和导航管理。
- 应用商店、NAS 导购与公开教程阅读。
- 卷备份、S3/WebDAV 存储、定时任务和通知。
- 自配模型的导航识别与 Compose YAML 生成。

不同发行版的可用能力以所选镜像及其授权为准。

## 部署

需要 Docker Engine 和 Docker Compose。默认配置使用完整版镜像 `coracoo/tradis:latest`。
下载本仓库 `client/docker-compose.yml` 和 `client/.env.example`，放入同一个安装目录：

```bash
cp .env.example .env
# 编辑 .env，填写 JWT_SECRET、ADMIN_PASSWORD 和宿主机项目路径。
docker compose pull
docker compose up -d
```

访问 `http://NAS地址:8080`，使用 `admin` 和你设置的初始密码登录。

| 配置 | 说明 |
| --- | --- |
| `JWT_SECRET` | 至少 32 字符的随机密钥，必填 |
| `ADMIN_PASSWORD` | 管理员初始密码，必填 |
| `PROJECT_ROOT` | 宿主机 Compose 项目绝对路径，必填 |
| `BACKUP_ROOT` | 宿主机备份绝对路径 |
| `TRADIS_PORT` | 面板端口，默认 8080 |
| `APPSTORE_CDN_URL` | 应用商店公开内容地址 |

数据库保存在安装目录的 `data` 中。请保留该目录和密钥，不要将 Docker socket 暴露到公网。
NAS 的 Compose 编辑器也可使用相同配置；需提供上述环境变量和绝对路径。

### 社区版（可选）

```bash
# 社区版使用独立目录、data 和 PROJECT_ROOT，不直接替换已有安装的镜像。
# 下载 client/docker-compose.community.yml 和 client/.env.community.example。
cp .env.community.example .env
# 填写密码、密钥、路径和社区版 Release 提供的 TRADIS_COMMUNITY_IMAGE（含 digest）。
docker compose -f docker-compose.community.yml pull
docker compose -f docker-compose.community.yml up -d
```

社区版仅提供本地管理，不调用官方 Server。应用商店、NAS 导购和教程使用配置的公开内容源。
社区版仅支持手动更新，没有应用内自动更新。

## 更新

更新前备份 `.env` 和 `data`，并查看对应镜像的发布说明。默认安装执行：

```bash
docker compose pull
docker compose up -d
```

社区版更新 `.env` 中的发布镜像 digest 后，使用 `-f docker-compose.community.yml` 执行同样操作。

## 从源码构建社区版

```bash
cd client
cp .env.community.example .env
# 填写密码、密钥和路径，将 TRADIS_COMMUNITY_IMAGE 设置为 tradis-community:local。
docker compose -f docker-compose.community.yml -f docker-compose.community.build.yml up -d --build
```

源码更新后重新构建。源码构建的镜像与公开发行镜像的 digest 可以不同。

## 授权与反馈

- 本仓库原创源码采用 **AGPL-3.0-only**，完整条款见 LICENSE；第三方依赖和素材遵循各自许可证。
- AGPL 允许商业使用，分发及提供网络服务时须遵守对应源码义务。
- 预构建镜像按照对应发行版本的授权条款使用，本仓库源码许可证不授予未公开代码的使用权。
- 问题可提交至本仓库 Issues；请勿公开密码、Token、数据库或未脱敏日志。
