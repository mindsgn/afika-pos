const { withProjectBuildGradle } = require("expo/config-plugins");

// expo-constants runs `scripts/getAppConfig.js` from the `createExpoConfig` Gradle
// task. That script calls `@expo/env`.load() before any mode is set. With
// NODE_ENV unset, Expo logs an error and skips `.env.production`. The JS bundle
// from `expo export:embed` already sets NODE_ENV from `--dev`; this covers the
// native app.config step. An explicit NODE_ENV is left unchanged.
const MARKER = "afikaReleaseNodeEnv";

const SNIPPET = `
// ${MARKER}: set NODE_ENV for expo-constants' createExpoConfig task.
gradle.projectsEvaluated {
    def existing = System.getenv("NODE_ENV")
    if (existing != null && !existing.isEmpty()) {
        return
    }
    def requested = gradle.startParameter.taskNames.join(" ").toLowerCase()
    def nodeEnv = requested.contains("release") ? "production" : "development"
    rootProject.subprojects { sub ->
        def configTask = sub.tasks.findByName("createExpoConfig")
        if (configTask != null) {
            configTask.environment("NODE_ENV", nodeEnv)
        }
    }
}
`;

function withAndroidReleaseNodeEnv(config) {
  return withProjectBuildGradle(config, (config) => {
    if (config.modResults.contents.includes(MARKER)) {
      return config;
    }
    config.modResults.contents += `\n${SNIPPET}`;
    return config;
  });
}

module.exports = withAndroidReleaseNodeEnv;
