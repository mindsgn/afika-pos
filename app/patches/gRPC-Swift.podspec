# Local override of the upstream `gRPC-Swift` podspec.
#
# Why: `@buildonspark/spark-sdk` depends on the CocoaPods `gRPC-Swift` pod, whose
# module is named `GRPC`. FirebaseFirestore (via `gRPC-Core`) builds a framework
# named `grpc`. CocoaPods' duplicate-name check is case-insensitive, so with
# `use_frameworks!` both pods end up producing a `grpc`/`GRPC` framework and
# `pod install` fails with:
#
#   The 'Pods-AFIKA' target has frameworks with conflicting names: grpc.
#
# Renaming this module to `GRPC_Swift` keeps the two frameworks distinct.
# `gRPC-Core` must not be renamed: `<grpc/...>` includes (used by gRPC-C++ and
# Firestore) resolve through the `grpc.framework` name itself.
#
# `node_modules/@buildonspark/spark-sdk/ios/SparkGrpcModule.swift` is patched to
# `import GRPC_Swift` (see patches/@buildonspark+spark-sdk+0.13.0.patch).
#
# Everything else mirrors the upstream 1.8.0 podspec.
Pod::Spec.new do |s|
  s.name         = 'gRPC-Swift'
  s.module_name  = 'GRPC_Swift'
  s.version      = '1.8.0'
  s.license      = { :type => 'Apache 2.0', :file => 'LICENSE' }
  s.summary      = 'Swift gRPC code generator plugin and runtime library'
  s.homepage     = 'https://www.grpc.io'
  s.authors      = { 'The gRPC contributors' => 'grpc-packages@google.com' }
  s.swift_versions = '5.4'
  s.platforms    = { :ios => '10.0', :osx => '10.12', :tvos => '10.0', :watchos => '6.0' }

  s.source       = { :git => 'https://github.com/grpc/grpc-swift.git', :tag => '1.8.0' }
  s.source_files = 'Sources/GRPC/**/*.{swift,c,h}'

  s.dependency 'CGRPCZlib', '1.8.0'
  s.dependency 'Logging', '>= 1.4.0', '< 2.0.0'
  s.dependency 'SwiftNIO', '>= 2.32.0', '< 3.0.0'
  s.dependency 'SwiftNIOExtras', '>= 1.4.0', '< 2.0.0'
  s.dependency 'SwiftNIOHTTP2', '>= 1.22.0', '< 2.0.0'
  s.dependency 'SwiftNIOSSL', '>= 2.14.0', '< 3.0.0'
  s.dependency 'SwiftNIOTransportServices', '>= 1.11.1', '< 2.0.0'
  s.dependency 'SwiftProtobuf', '>= 1.19.0', '< 2.0.0'

  s.swift_version = '5.4'
end
