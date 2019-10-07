module.exports = {
	env: {
		//browser: true,
		node: true,
		es6: true,
	},
	extends: 'eslint:recommended',
	globals: {
		Atomics: 'readonly',
		SharedArrayBuffer: 'readonly',
	},
	parserOptions: {
		ecmaVersion: 10,
		sourceType: 'module',
	},
	rules: {
		'no-console': 'warn',
		'no-unused-vars': ['error', {'vars': 'all', 'args': 'none'}],
	},
}
