/** @type {import('next').NextConfig} */
const nextConfig = {
  // Wagmi/RainbowKit pulls in transport libraries (WalletConnect, MetaMask SDK)
  // that have optional Node-only peer deps:
  //   - pino-pretty (only used in dev logging)
  //   - @react-native-async-storage/async-storage (only used in RN apps)
  //
  // We tell webpack to ignore them so our browser bundle compiles clean.
  webpack(config, { webpack }) {
    config.plugins = config.plugins || [];
    config.plugins.push(
      new webpack.IgnorePlugin({
        resourceRegExp:
          /^(pino-pretty|@react-native-async-storage\/async-storage)$/,
      }),
    );
    return config;
  },
};

export default nextConfig;
