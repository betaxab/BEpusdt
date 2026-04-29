// 获取法币对应的旗帜emoji
const getFiatFlag = (fiat: string) => {
  const fiatFlagMap: Record<string, string> = {
    CNY: "🇨🇳",
    USD: "🇺🇸",
    JPY: "🇯🇵",
    EUR: "🇪🇺",
    GBP: "🇬🇧"
  };
  return fiatFlagMap[fiat] || "🌍";
};

export const getCryptoColor = (crypto: string): string => {
  const colors: Record<string, string> = {
    USDT: "green",
    USDC: "blue",
    CNYE: "magenta",
    TRX: "red",
    ETH: "purple",
    BNB: "orange"
  };
  return colors[crypto] || "gray";
};

export { getFiatFlag };
