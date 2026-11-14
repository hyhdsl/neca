// Package ports 定义 neca 核心能力的稳定边界（ports & adapters 里的 ports 侧）。
//
// 定位：
//   - 只放"核心逻辑依赖、但实现可替换"的接口与最小数据类型；
//   - 不依赖任何实现包，也不依赖 cmd/*、internal/cli；
//   - 具体实现放在 internal/core 的领域文件或未来新增的 adapter 包中，
//     通过构造函数注入。
//
// 为什么单独成包：若接口散落在 internal/core 各文件，容易出现
// core -> 子包 -> core 的循环依赖；一个只含接口与数据类型的叶子包
// 可以让实现方和消费方都安全引用。
//
// 稳定性：本包位于 internal/，不承诺对仓库外兼容。v0.1.0 之前接口可演进，
// 但每次演进必须同步更新开发文档 §3.1 与测试中的假实现，避免契约漂移。
//
// 当前已定契约：
//   - IndexStore：索引读取与锁内读-改-写（index.go）；
//   - Source / Resolver：配置来源与优先级合并（config.go）。
//
// 候选扩展点（Engine / AuditSink / Bundler / Signer / Notifier）只在
// 文档 §3.1 登记，等 M1/M2 出现第一个真实实现时再抽象，避免空接口。
package ports
