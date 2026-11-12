// Learn more https://docs.expo.io/guides/customizing-metro
const { getDefaultConfig } = require("expo/metro-config");

/** @type {import('expo/metro-config').MetroConfig} */
const config = getDefaultConfig(__dirname);

// Secure the upstream resolver safely, falling back to context's native resolver if undefined
const upstreamResolveRequest =
  config.resolver.resolveRequest ||
  ((context, moduleName, platform) => {
    return context.resolveRequest(context, moduleName, platform);
  });

config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (moduleName === "jose") {
    const ctx = {
      ...context,
      unstable_conditionNames: ["browser"],
    };
    return upstreamResolveRequest(ctx, moduleName, platform);
  }

  // Always route back through the secured resolution chain
  return upstreamResolveRequest(context, moduleName, platform);
};

config.transformer.babelTransformerPath = require.resolve(
  "react-native-svg-transformer"
);

config.resolver.assetExts = config.resolver.assetExts.filter(
  (ext) => ext !== "svg"
);

config.resolver.sourceExts.push("svg");

config.resolver.sourceExts.push("sql");
config.resolver.extraNodeModules = {
  ...(config.resolver.extraNodeModules || {}),
  events: require.resolve("events"),
};

module.exports = config;
