const levelOrder = { debug: 10, info: 20, warn: 30, error: 40 };
export function isLevelEnabled(configured, candidate) {
    return levelOrder[candidate] >= levelOrder[configured];
}
//# sourceMappingURL=logger.js.map