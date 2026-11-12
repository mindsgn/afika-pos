const { withPodfile } = require('expo/config-plugins');

// `@buildonspark/spark-sdk` pulls in the CocoaPods `gRPC-Swift` pod (Swift module
// `GRPC`), while Firebase's `gRPC-Core` builds a framework named `grpc`. Under
// `use_frameworks!` CocoaPods compares those names case-insensitively and fails
// with "The 'Pods-AFIKA' target has frameworks with conflicting names: grpc."
//
// We point the pod at a local podspec that renames the Swift module to
// `GRPC_Swift` (patches/gRPC-Swift.podspec). The matching `import GRPC` in
// spark-sdk is rewritten via patches/@buildonspark+spark-sdk+0.13.0.patch,
// which is applied by the `postinstall` script.
//
// `gRPC-Core` must NOT be renamed: gRPC-C++ and Firestore use `<grpc/...>`
// includes, which resolve through the `grpc.framework` name itself.
const POD_LINE = "pod 'gRPC-Swift', :podspec => '../patches/gRPC-Swift.podspec'";

const ANNOTATION = [
  "  # Keep the gRPC-Swift module distinct from Firebase's grpc framework.",
  "  # Injected by plugins/withGrpcSwiftModuleFix.js; see that file for details.",
  `  ${POD_LINE}`,
].join('\n');

module.exports = function withGrpcSwiftModuleFix(config) {
  return withPodfile(config, (config) => {
    const contents = config.modResults.contents;

    if (contents.includes(POD_LINE)) {
      return config;
    }

    if (!contents.includes('use_expo_modules!')) {
      throw new Error(
        '[withGrpcSwiftModuleFix] Could not find "use_expo_modules!" in the generated iOS Podfile.'
      );
    }

    config.modResults.contents = contents.replace(
      'use_expo_modules!',
      `use_expo_modules!\n\n${ANNOTATION}`
    );

    return config;
  });
};
