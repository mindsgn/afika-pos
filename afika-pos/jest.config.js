module.exports = {
  testEnvironment: "node",
  testMatch: ["**/*.test.ts"],
  moduleNameMapper: {
    "^@/(.*)$": "<rootDir>/src/$1",
  },
  transformIgnorePatterns: ['node_modules/(?!expo/virtual/)'],
  transform: {
    '^.+\\.[tj]sx?$': ['babel-jest', { presets: ['babel-preset-expo'] }],
  },
};
