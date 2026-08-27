// Package brand 集中 HaoVPN 产品名、二进制名、默认 TUN 名与环境变量名。
//
// 关键文件：brand.go — DefaultTunName、WintunPool、CredDirName 等常量。
//
// 上游：cmd/*、tun、clientapp、winnet、singleinstance。
// 下游：无 internal 依赖。
// 不变量：禁止在业务代码散落硬编码产品字符串；改品牌只改本包。
package brand
