# 构建与发布校验

使用 Docker 镜像部署时，无需手动运行本目录脚本。这些脚本由源码构建和发布工作流使用。

| 文件 | 用途 |
| --- | --- |
| check-community-dependencies.sh | 检查后端依赖是否超出社区版功能范围 |
| community-export.json | 上述依赖检查的规则，不含运行配置 |
| check-community-export-secrets.sh | 检查源码是否误带密钥、凭据或运行数据 |
| check-community-export-sync.sh | 校验文件清单、哈希与待发布源码的一致性 |
| check-community-release.sh | 发布前检查版本、许可证和源码完整性 |

前端的 `client/frontend/scripts/` 负责扫描构建产物中的非社区版入口，
通过 `npm run verify:community` 调用。这些都是构建工具，不参与应用运行。
