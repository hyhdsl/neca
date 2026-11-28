# neca 发布签名公钥

本目录只保存用于验证 neca Release 资产签名的公钥，不保存任何私钥或口令。

- 公钥：`neca-release-signing.asc`
- 算法：Ed25519
- 指纹：E514 EB7C 153A A24A 3E42 41F8 46CC 03EE 21F6 6795
- 用途：签名 `checksums.txt`，覆盖全部发布二进制；私钥只存在于 GitHub 受保护的 `release-signing` environment。
- v1.5.0 的历史签名使用既有 neca GPG 密钥；v1.5.1 起使用本专用发布密钥。
- 轮换：密钥到期或私钥暴露时，先发布新公钥和指纹，再切换 workflow；旧公钥保留用于历史 Release 验证。

验证方式见 [简体中文安装文档](../docs/help/zh/install.md) 与 Release 说明。
