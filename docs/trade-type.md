# 交易类型

有时候各方对接插件的交易类型更新不一定及时，但这里列出来的交易类型是最新的，可以以这里为准进行调整。<br>
分别对应的是区块网络，以及对应支持的交易类型：

|    **网络**    |    **USDT**     |    **USDC**     |     **其它**     |
|:------------:|:---------------:|:---------------:|:--------------:|
|     Tron     |  `usdt.trc20`   |  `usdc.trc20`   |   `tron.trx`   |
|   Ethereum   |  `usdt.erc20`   |  `usdc.erc20`   | `ethereum.eth` |
|   Polygon    | `usdt.polygon`  | `usdc.polygon`  |                |
|     BSC      |  `usdt.bep20`   |  `usdc.bep20`   |   `bsc.bnb`    |
|    Aptos     |  `usdt.aptos`   |  `usdc.aptos`   |                |
|    Solana    |  `usdt.solana`  |  `usdc.solana`  |                |
|   X-Layer    |  `usdt.xlayer`  |  `usdc.xlayer`  |                |
| Arbitrum-One | `usdt.arbitrum` | `usdc.arbitrum` |                |
|     Base     |                 |   `usdc.base`   |                |
|    Plasma    |  `usdt.plasma`  |                 |                |
|     Ton      |   `usdt.ton`    |                 |   `ton.gram`   |

---

## 第三方通道

| **通道** | **交易类型** | **说明** |
|:------:|:----------:|:------|
| 支付宝 | `alipay.mck` | 支付宝通道 |
| 京东支付 | `duolabao.qr` | 哆啦宝动态二维码 |
| Stripe | `stripe.alipay` | Stripe Checkout，仅显示支付宝 |
| Stripe | `stripe.wechatpay` | Stripe Checkout，仅显示微信支付 |
| Stripe | `stripe.card` | Stripe Checkout，仅显示银行卡 |
| Stripe | `stripe.all` | Stripe Checkout，不限制页面内付款类型 |
