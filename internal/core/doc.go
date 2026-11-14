// Package core 是 neca 的唯一业务实现层（开发文档 §3 架构约束 1）。
//
// 约束（后续所有 core 子包都必须遵守）：
//   - 不依赖展示层（cmd/*、internal/cli），也不直接向 stdout 打印；
//     所有操作以结构化结果返回，由前端决定渲染为人类可读文本还是 JSON；
//   - 所有外部命令统一经 core/proc 执行，其余包禁止直接使用 os/exec；
//   - 路径与开关只在 core/config 定义一次，避免散落的魔法常量。
//
// 可替换能力（索引存储、配置来源等）的接口统一定义在 internal/core/ports，
// 扩展点清单与新增步骤见开发文档 §3.1；实现通过构造函数注入，不在包内
// 直接实例化，保证测试可替换、未来可接新后端。
//
// M0 只建立包骨架，不写业务代码。后续依次落地：
//
//	M1：config / proc / openssl / index（只读）/ meta / table / csv
//	M2：pki / crl / engine / audit / bundle
package core
