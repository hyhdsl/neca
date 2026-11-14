# vpn-cert-manager.sh 使用与二次开发说明

适用版本：2026-11-15 版脚本（约 506 行），文件位置 vpn/ipsec/vpn-cert-manager.sh（服务器：/opt/ipsec/scripts/vpn-cert-manager.sh）。
本文档只描述该脚本本身；服务端 VPN 部署见 vpn/ipsec-vpn-deployment.md。

---

## 1. 概述

vpn-cert-manager.sh 是一个面向 strongSwan IKEv2（X.509 证书认证）的用户证书生命周期管理脚本，同时提供交互式向导和命令行两种用法。

主要能力：

| 能力 | 说明 |
|---|---|
| 签发新证书 | 选择平台（Windows / Android / iOS，可多选），生成证书、p12、按平台打包 |
| 吊销证书 | 写入 CA 索引、重建 CRL、重启 VPN 容器加载 CRL |
| 恢复吊销 | 把索引中的 R 改回 V，重建 CRL、重启容器 |
| 重签发旧证书 | 旧证书归档 + 标记吊销，同 CN 生成新证书 |
| 增加平台包 | 用已有证书补生成其它平台目录（不重签） |
| 删除证书 | 删除证书/私钥/p12/元数据并从索引移除，同时重建 CRL |
| 导出 CSV | 已签发、已吊销两份列表（UTF-8 BOM，Excel 直接打开） |
| 索引同步 | 为手工放入的 .crt 补索引和元数据 |
| p12 自检 | 每次导出后检查 3DES、证书数量、密码长度；--verify 批量检查 |

设计目标：单文件、可迁移、可二次开发。所有路径与参数集中在脚本头部变量区，可用环境变量覆盖。

---

## 2. 运行环境

| 依赖 | 用途 | 缺失时 |
|---|---|---|
| bash 4+ | 脚本本身（数组、here-string、ANSI-C 引用） | 无法运行 |
| openssl 3.x | 签发、p12、CRL、解析证书 | init_env 直接报错退出 |
| awk / sed / date / mktemp / install | 索引改写、文本处理、日期换算 | 报错退出 |
| docker（可选） | 重启容器以加载新 CRL | 只告警，需手动重启 |
| python3（可选） | 生成 iOS .mobileconfig；--list 表格按显示宽度对齐 | 跳过 iOS 包；表格回退 column -t 或原样输出 |
| column（可选） | 无 python3 时的表格回退 | 原样输出 TSV |

脚本以服务端 root 运行最方便（需读 CA 私钥、写 /opt/ipsec、重启容器）。只读操作（--list、--export-*）普通用户也可，但需对相关目录有读权限。

---

## 3. 快速开始

    chmod +x vpn-cert-manager.sh

    ./vpn-cert-manager.sh                 # 交互向导
    ./vpn-cert-manager.sh --list          # 查看证书
    ./vpn-cert-manager.sh --issue HYH --platforms windows,android --days 365
    ./vpn-cert-manager.sh --verify        # 检查所有客户包的 p12 兼容性

首次运行会自动创建缺失的目录、index.txt、crlnumber、openssl.cnf（前提是 CA_CERT 存在）。

---

## 4. 命令行参考

通用参数：

| 参数 | 适用动作 | 含义 |
|---|---|---|
| --issue CN | issue | 签发新证书，CN 必填 |
| --revoke CN | revoke | 吊销 |
| --unrevoke CN | unrevoke | 取消吊销 |
| --reissue CN | reissue | 重签发（归档并吊销旧证书） |
| --add-platform CN | add-platform | 为已有证书补平台包 |
| --set-platforms CN | set-platforms | 只改元数据里的平台标注（接受任意标签，如 test） |
| --delete CN | delete | 删除证书及索引；非交互需加 --yes |
| --export-issued [--out F] | export-issued | 导出已签发 CSV，默认写入 EXPORT_DIR/certs-issued-时间戳.csv |
| --export-revoked [--out F] | export-revoked | 导出已吊销 CSV，同上 |
| --platforms a,b | issue/reissue/add-platform | 平台列表，逗号分隔；支持别名 win/w、and/a/安卓、ios/apple/i/苹果 |
| --days N | issue/reissue | 有效期天数，默认 DEFAULT_DAYS（365） |
| --out F | export-* | 输出文件路径 |
| --yes / -y | delete | 跳过确认 |
| --list、--sync、--verify | — | 列表 / 补索引 / p12 自检 |
| -h / --help | — | 用法提示 |

注意：

- 没有动作参数时进入交互向导；无法识别的第一个位置参数会被当作 CN（便于 ./script CN 风格）。
- --set-platforms 的 --platforms 不做规范化（保留原样标签）；其余动作会经 norm_platforms 过滤成 windows/android/ios。
- --delete 在非 --yes 时用 confirm 读 y/N；无 TTY 会 EOF 失败，自动化场景务必带 --yes。
- 导出动作只输出 [OK] 行；需要路径可在 shell 中自行拼（或用 --out）。

---

## 5. 交互向导

    ======== VPN 证书管理 ========
       1) 签发新证书（选择平台）
       2) 吊销证书
       3) 恢复已吊销证书
       4) 重签发旧证书
       5) 为已有证书增加平台包
       6) 导出已签发证书列表 (CSV)
       7) 导出已吊销证书列表 (CSV)
       8) 查看证书列表
       9) 同步索引 / 修复元数据
      10) 删除证书（危险）
       0) 退出

- 1/4/5 会先让选平台：1) Windows 2) Android 3) iOS 4) 全部，可输入 1,2 多选。
- 4 会二次确认“旧证书将归档并吊销”。
- 10 会二次确认“删除 CN 并从索引/CRL 移除”。
- 菜单循环执行，0/q/Q 退出。

---

## 6. 配置变量（迁移点）

全部为“环境变量优先、否则用默认值”的写法，可 export 覆盖：

| 变量 | 默认值 | 说明 |
|---|---|---|
| PKI_DIR | /opt/ipsec/pki | CA 证书/私钥、客户端证书/私钥、ca/、meta/、archive/ 的根 |
| CA_CERT | $PKI_DIR/ca.crt | CA 证书（PEM） |
| CA_KEY | $PKI_DIR/ca.key | CA 私钥（PEM） |
| CADB_DIR | $PKI_DIR/ca | CA 数据库目录（index/crlnumber/openssl.cnf） |
| INDEX_FILE | $CADB_DIR/index.txt | 索引文件 |
| CRLNUM_FILE | $CADB_DIR/crlnumber | CRL 序号文件 |
| CA_CONF | $CADB_DIR/openssl.cnf | openssl ca 配置（不存在时自动生成） |
| CRL_DIR | /opt/ipsec/swanctl/x509crl | CRL 输出目录（strongSwan 读取位置） |
| CRL_FILE | $CRL_DIR/ca.crl.pem | CRL 文件 |
| CLIENTS_DIR | /opt/ipsec/clients | 客户包根目录，每 CN 一个子目录 |
| META_DIR | $PKI_DIR/meta | 每 CN 一个 .env 元数据 |
| ARCHIVE_DIR | $PKI_DIR/archive | 重签发/删除时的归档 |
| EXPORT_DIR | /opt/ipsec/exports | CSV 默认输出目录 |
| CONTAINER | ipsec-vpn | 需要重启的 strongSwan 容器名 |
| DOCKER_BIN | docker | docker 可执行文件（可换 podman 等兼容命令） |
| SERVER_ID | vpn.example.com | 服务端 IKE 标识符，写入证书 SAN 与各平台配置 |
| CA_CN | ExampleOrg VPN CA | 预留变量；Windows/iOS 模板里 CA 名称是硬编码字符串，改 CA 名时需同步模板 |
| DEFAULT_DAYS | 365 | 默认有效期 |
| KEY_BITS | 3072 | RSA 位数 |
| ORG | ExampleOrg | 证书 subject 的 O 字段 |

迁移示例：

    export PKI_DIR=/srv/vpn/pki
    export CLIENTS_DIR=/srv/vpn/clients
    export CRL_FILE=/srv/vpn/swanctl/x509crl/ca.crl.pem
    export CONTAINER=my-ipsec
    export SERVER_ID=vpn.example.com
    export ORG=Example
    ./vpn-cert-manager.sh --list

---

## 7. 目录与数据模型

### 7.1 PKI 目录

    $PKI_DIR/
    ├─ ca.crt / ca.key            # CA
    ├─ ca.srl                     # 下一个序列号（openssl x509 -req -CAcreateserial 使用）
    ├─ ca/
    │  ├─ index.txt               # 证书状态库
    │  ├─ crlnumber               # CRL 序号
    │  └─ openssl.cnf             # openssl ca 配置（gencrl 用）
    ├─ <CN>.crt / <CN>.key        # 客户端证书/私钥（当前有效那一张）
    ├─ <CN>.csr / <CN>.ext        # 签发过程的临时文件（可删）
    ├─ meta/<CN>.env              # 平台等元数据
    └─ archive/<CN>-<serial>.crt|.key   # 重签发/删除前的历史副本

### 7.2 索引 index.txt

每行 6 个字段，制表符分隔：

    状态 <TAB> 到期(YYMMDDHHMMSSZ) <TAB> 吊销时间(可空) <TAB> 序列号 <TAB> 文件名(固定 unknown) <TAB> 主题(openssl 文本格式)

- 状态：V 有效，R 已吊销。
- index_add 写入 V 行；idx_set 按序列号改状态/吊销时间；idx_del 按序列号删行。
- CRL 由 openssl ca -gencrl 依据本文件生成，所以已吊销的 R 行必须保留，删掉会让 CRL 丢失该吊销记录（被吊销证书重新可用）。
- 服务端证书（vpn.example.com）不进入索引；sync_index 与 list_certs 都会跳过 ca.crt、server.crt。

### 7.3 元数据 meta/<CN>.env

    CN=<CN>
    PLATFORMS=windows,android,ios
    CREATED=YYYY-MM-DD
    EXPIRES=<openssl 格式到期时间>
    SERIAL=<序列号>

仅用于展示平台、导出 CSV；证书本体不依赖它。sync_index 会给缺失的 CN 补 unknown 平台。

### 7.4 客户包 $CLIENTS_DIR/<CN>/

    <CN>/ca.crt                  # 供客户端信任的 CA
    <CN>/<CN>.crt                # 证书 PEM（核对指纹）
    <CN>/<CN>.p12                # 客户端证书+私钥（3DES，仅客户端证书）
    <CN>/<CN>.p12.pass           # 12 位字母数字密码（无换行）
    <CN>/Windows/                # install.ps1 / uninstall.ps1 / connect.ps1
                                 # 安装-<CN>.cmd / 卸载-<CN>.cmd / 连接-<CN>.cmd
                                 # <CN>.p12 / pass / ca.crt / 使用说明.txt
    <CN>/Android/                # <CN>.p12 / pass / ca.crt / 使用说明.txt
    <CN>/iOS/                    # <CN>.mobileconfig / （可选说明）

平台包按需生成：只签发 windows,android 时不会创建 iOS 目录。

### 7.5 导出 CSV

两份独立文件，均带 UTF-8 BOM，字段：

- 已签发：CN, 序列号, 状态, 生效, 到期, 剩余天数, SAN, 平台, SHA256指纹, P12路径
- 已吊销：CN, 序列号, 状态, 生效, 到期, 剩余天数, SAN, 平台, SHA256指纹, 说明

已吊销且文件已删除的历史条目会以“已删除 + 证书文件已删除”输出，保证条数可与 CRL 对照。

---

## 8. 证书规范（脚本产出）

| 项 | 规则 |
|---|---|
| CN | 仅允许 A-Z a-z 0-9 . _ -，不允许空格/中文 |
| 私钥 | RSA KEY_BITS（默认 3072），文件权限 600 |
| Subject | /C=CN/O=<ORG>/CN=<CN> |
| 扩展 | basicConstraints=critical,CA:FALSE；keyUsage=digitalSignature,keyEncipherment；extendedKeyUsage=clientAuth；subjectAltName=DNS:<SERVER_ID>,DNS:<CN>；SKI/AKI |
| 有效期 | --days（默认 365） |
| 签名 | sha256，CA 为 CA_CERT/CA_KEY |
| p12 | -keypbe PBE-SHA1-3DES -certpbe PBE-SHA1-3DES -macalg sha1；只含客户端证书（不带 CA 链）；文件名 <CN>.p12 |
| 密码 | 12 位 A-Za-z0-9，写入 <CN>.p12.pass（无换行；早期脚本生成的包可能带换行） |
| Windows 配置名 | ExamplePanel-VPN-<CN>，IKEv2 + MachineCertificate + SplitTunneling |

为什么强制 3DES + 单证书：MIUI 自带证书安装器对 OpenSSL 3 默认的 PBES2/AES-256 + CA 链会卡在“提取证书”。verify_p12 会在每次导出后检查并告警。

---

## 9. 核心逻辑与状态机

### 9.1 签发 issue_cert

1. 校验 CN（非空、字符集、$PKI_DIR/<CN>.crt 不存在）；
2. openssl genrsa → req -subj → 写 .ext → x509 -req -CAcreateserial -days -sha256；
3. index_add 写 V 行；meta_write 写平台/日期/序列号；
4. export_p12（含 verify_p12 自检）；
5. bundle_all 按平台生成目录。

失败点均 die（非 0 退出），已生成的文件可能残留，重跑前需清理或改用 --reissue。

### 9.2 吊销 / 恢复

- 吊销：idx_set <serial> R <UTC 时间> → gencrl → reload_engine。
- 恢复：idx_set <serial> V → gencrl → reload_engine。
- reload_engine：docker restart <CONTAINER>，然后最多 40 秒轮询 docker exec <CONTAINER> swanctl --stats；找不到容器时只告警。
- 重要：strongSwan 只在启动时加载 x509crl 目录；实测 swanctl --load-creds 不会刷新已缓存的 CRL，所以必须重启。重启会断开所有在线 VPN 会话。

### 9.3 重签发 reissue_cert

1. 若当前 <CN>.crt 存在：复制到 archive/<CN>-<serial>.crt/.key；
2. 若索引为 V → 置 R（保留历史）；
3. 删除当前 crt/key/csr/ext；
4. 调 issue_cert 生成新证书（新序列号，同 CN）；
5. gencrl + reload_engine。

结果：索引里同一 CN 可能同时有 R（旧）与 V（新），列表只显示当前文件对应的 V。

### 9.4 删除 delete_cert

- 删除当前 crt 对应的索引行；删除 archive/<CN>-*.crt 对应的索引行；
- 删除 <CN>.crt/.key/.csr/.ext、clients/<CN>/、meta/<CN>.env、archive/<CN>-*；
- gencrl + reload_engine。

注意：它会删除该 CN 的 R 行（语义是“彻底移除”），这与“吊销保留 R”不同；对已吊销证书不要用 --delete，否则 CRL 会丢条目。

### 9.5 增加平台 add_platform

- 复用现有证书/p12（缺失则 export_p12），只重复生成指定平台目录；
- 合并 meta 的 PLATFORMS（merge_platforms）。

### 9.6 索引同步 sync_index

扫描 $PKI_DIR/*.crt（跳过 ca/server），把不在索引中的证书补为 V，并补缺失的 meta（平台 unknown）。适合从旧环境迁移或手工放入证书后使用。

### 9.7 列表 list_certs

输出 TSV 后交给 format_table：python3 用 unicodedata.east_asian_width 按显示宽度补空格（中文占 2 列），表头下加分隔线；另追加“已吊销但文件已删除”的索引条目。无 python3 时退回 column -t，再退回原样 TSV。

---

## 10. 函数索引（二次开发）

| 函数 | 作用 | 关键依赖 |
|---|---|---|
| ok/info/warn/die/need/ask/confirm | 输出与交互原语；die 退出 1 | — |
| init_env | 建目录、补 index/crlnumber/openssl.cnf、校验 CA | — |
| cert_serial/exp_idx/subj/cn/san/fp/days | 从证书提取字段 | openssl |
| index_add/idx_status/idx_set/idx_del | 索引读写 | awk/mktemp |
| find_cert_by_serial | 在 archive 与 pki 中按序列号找证书 | openssl |
| gencrl | 生成并落盘 CRL | openssl ca |
| reload_engine | 重启容器并等待 swanctl 可用 | docker |
| meta_write/meta_get/meta_set/meta_file | 元数据读写 | sed/awk |
| norm_platforms/merge_platforms | 平台规范化/合并 | — |
| gen_pass | 12 位字母数字密码 | /dev/urandom、base64 |
| export_p12 | 导出 p12 + ca.crt + crt，并调 verify_p12 | openssl |
| verify_p12 | 检查 3DES/证书数/密码长度 | openssl |
| bundle_all/bundle_platform | 平台分发 | — |
| bundle_windows | 生成 PowerShell/cmd/p12 包 | sed、heredoc 模板 |
| bundle_android | 生成 p12 + 中文说明 | — |
| bundle_ios | 生成 .mobileconfig（CA+身份+IKEv2） | python3、plistlib |
| issue_cert | 签发全流程 | openssl |
| revoke_cert/unrevoke_cert | 吊销/恢复 | gencrl、reload_engine |
| reissue_cert | 归档+重签 | issue_cert 等 |
| add_platform/set_platforms | 补平台/改标注 | bundle_all、meta |
| delete_cert | 彻底删除 | idx_del、gencrl、reload_engine |
| sync_index | 补索引/元数据 | — |
| format_table/list_certs | 列表渲染 | python3（可选） |
| csvq/row_for_cert | CSV 行生成 | openssl、date |
| export_issued/export_revoked | CSV 导出 | row_for_cert |
| select_platforms | 交互选平台 | — |
| wizard | 交互主循环 | 上述动作 |
| 末尾 CLI 解析 | 参数解析与分发 | — |

---

## 11. 二次开发指南

### 11.1 新增平台（例如 linux）

1. bundle_platform() 的 case 增加分支：linux) bundle_linux "$cn" "$d";;
2. 实现 bundle_linux()：生成 $CLIENTS_DIR/<CN>/Linux/ 内容（至少 p12 + pass + ca.crt + 说明）。
3. norm_platforms() 增加别名映射（如 linux|l → linux）。
4. select_platforms() 增加编号（如 5) Linux）。
5. 若平台需要额外 SAN，在 issue_cert 的 .ext 模板里加 DNS:<...>。

### 11.2 修改证书扩展 / 有效期 / 密钥

- 扩展：issue_cert 内 cat > "$PKI_DIR/$cn.ext" <<EOF ... EOF 段。
- 默认有效期：改 DEFAULT_DAYS 或调用时 --days。
- 密钥长度：改 KEY_BITS。
- 签名算法：openssl x509 -req ... -sha256。

### 11.3 修改 Windows / iOS 模板

- Windows 模板是 bundle_windows() 里的三个 sed ... <<'PS1' heredoc，占位符：__CN__、__PROFILE__、__SERVER__。
- 模板必须保持纯 ASCII：PowerShell 5.1 在无 BOM 时按 ANSI 读脚本，中文会乱码报语法错误。需要中文时另存为 UTF-8 with BOM，并在生成后确认编码。
- iOS 模板是 bundle_ios() 内嵌的 python heredoc；LocalIdentifier 取 CN，RemoteAddress/RemoteIdentifier 取 SERVER_ID，OverridePrimary=0（拆分隧道）。

### 11.4 修改 CSV

- 列头与顺序在 export_issued/export_revoked 的 echo 行；
- 值由 row_for_cert 生成（10 个 csvq 字段），增加列时同步改这两处；
- 别忘 printf '\xEF\xBB\xBF' 保留 BOM。

### 11.5 约定与注意事项

- 全局 set -uo pipefail（无 -e）：错误需显式 die/return；管道中 head 截断可能触发 SIGPIPE，脚本已用容错写法，新增管道时注意。
- 索引改写统一 mktemp + mv（原子替换）；不要并发运行本脚本（两个实例同时改 index 会互相覆盖）。
- CRL 相关动作都会重启容器（约 10 秒），批量操作时尽量合并（例如先手工改索引再 gencrl 一次）。
- 序列号统一大写十六进制；索引到期时间必须 YYMMDDHHMMSSZ（脚本用 date -u）。
- 删除类操作先 confirm；自动化必须 --yes。
- 所有 [OK]/[WARN]/[ERR] 前缀被下游脚本/人工识别，改文案时保留前缀。

### 11.6 与 strongSwan 的契约

- CRL 必须落在 CRL_DIR（容器内 /etc/swanctl/x509crl），且 charon 配置 load_crls = yes；
- 容器名默认 ipsec-vpn，可用 CONTAINER 覆盖；
- 重启后以 swanctl --stats 判定可用；若容器内没有 swanctl，需要替换 reload_engine 的就绪检测；
- 服务端 send_cert = always、local_ts = <VPN_SUBNET> 等与证书无关，不在本脚本职责内。

---

## 12. 迁移到其它主机

    # 假设新环境：CA 与目录结构可自定义
    export PKI_DIR=/data/vpn/pki
    export CLIENTS_DIR=/data/vpn/clients
    export CRL_DIR=/data/vpn/swanctl/x509crl
    export EXPORT_DIR=/data/vpn/exports
    export CONTAINER=vpn-server
    export SERVER_ID=vpn.example.com
    export ORG=Example
    export DEFAULT_DAYS=365

    mkdir -p "$PKI_DIR"
    cp ca.crt ca.key "$PKI_DIR"/          # 或重新生成 CA
    ./vpn-cert-manager.sh --sync           # 若已有旧证书文件，补索引
    ./vpn-cert-manager.sh --list

迁移注意：

- openssl.cnf 由脚本按新路径自动生成；若沿用旧 CA_CONF，里面的 dir/database 等路径需同步修改。
- 已有客户包里的 p12 密码文件格式（有无换行）不影响脚本；verify_p12 只看长度。
- 远程执行（ssh）时注意交互动作需要 TTY；自动化只调用带 --yes 的动作。
- 若容器不是 docker（如 podman），把 DOCKER_BIN 指向兼容命令；docker ps --format、docker exec ... swanctl --stats 需要兼容。

---

## 13. 故障排查

| 现象 | 可能原因 | 处理 |
|---|---|---|
| 找不到 CA 证书 | CA_CERT 路径不对 | 检查 PKI_DIR/CA_CERT，或先放好 CA |
| CRL 生成失败 | CA_CONF 路径/内容错、CA 私钥不可读、crlnumber 缺失 | 检查 $CADB_DIR/openssl.cnf 的 dir/database/crlnumber/certificate/private_key |
| 吊销后仍能连 | 容器未重启 / 未加载 CRL | docker restart $CONTAINER，确认 load_crls = yes；日志应出现 certificate was revoked |
| 未找到容器 | CONTAINER 名不对或非 docker | 用 docker ps 核对，或手动重启 strongSwan |
| iOS 包缺失 | 无 python3 | 安装 python3，或只发 Windows/Android |
| --list 不对齐 | 无 python3 且无 column | 安装 python3 或 util-linux |
| p12 自检 WARN（PBES2/AES） | 有人改了 export_p12 的 PBE 参数 | 恢复 -keypbe/-certpbe PBE-SHA1-3DES -macalg sha1 |
| 证书已存在 | 该 CN 已有 .crt | 用 --reissue 或先 --delete |
| 索引补充后列表出现未索引 | 证书没进索引且 sync 未跑 | 运行 --sync |
| 删除后 CRL 少条目 | 对已吊销证书执行了 --delete | 不该对 R 记录用 --delete；必要时手工保留 R 行 |

---

## 14. 回归测试清单

修改脚本后建议按顺序验证（用临时 CN，如 TMP01）：

    bash -n vpn-cert-manager.sh
    ./vpn-cert-manager.sh --list
    ./vpn-cert-manager.sh --issue TMP01 --platforms windows,android --days 7
    ./vpn-cert-manager.sh --verify
    ./vpn-cert-manager.sh --add-platform TMP01 --platforms ios
    ./vpn-cert-manager.sh --export-issued --out /tmp/i.csv   # 应含 TMP01
    ./vpn-cert-manager.sh --revoke TMP01                      # CRL 出现新序列号
    ./vpn-cert-manager.sh --export-revoked --out /tmp/r.csv   # 应含 TMP01
    ./vpn-cert-manager.sh --unrevoke TMP01                    # CRL 移除
    ./vpn-cert-manager.sh --reissue TMP01 --platforms windows,android --days 7
    ./vpn-cert-manager.sh --delete TMP01 --yes                # 文件/索引/CRL 清理

Windows 包额外检查（在 Windows 上）：

    $bytes = [IO.File]::ReadAllBytes('install.ps1')
    ($bytes | Where-Object { $_ -gt 127 }).Count   # 应为 0

---

## 15. 已知限制

- 单证书同一时间只能一台设备在线（IKEv2 同 ID + INITIAL_CONTACT 会互踢）；需要多设备同时在线请按设备签发不同 CN。
- 吊销/恢复/重签发/删除都会重启容器，断开所有 VPN 会话。
- 不支持 CRL 增量、OCSP、证书策略（policy）等高级 PKI 功能。
- 不支持并发执行；索引与元数据没有文件锁。
- 服务端证书（server.crt）不在管理范围内，sync_index 会跳过。
- CA_CN 变量目前未实际使用（Windows/iOS 模板中硬编码 CA 名称），改名 CA 时需同步模板。
- 密码文件是否带换行由生成方式决定（12 字节无换行 / 13 字节带换行），不影响脚本，但手工输入时注意不要多输换行。

---

## 16. 变更记录（脚本）

| 日期 | 变更 |
|---|---|
| 2026-11-14 | 初版：签发/吊销/恢复/重签/加平台/导出/列表；CRL + 容器重启 |
| 2026-11-14 | 列表改为按显示宽度对齐；菜单改逐行；新增 --verify 与导出后 p12 自检 |
| 2026-11-15 | 补充 Windows 包使用说明；文档独立成本文件 |
