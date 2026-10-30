# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability in grepai, please report it responsibly.

### How to Report

1. **Do NOT** open a public issue for security vulnerabilities
2. Email your findings to: yoan.bernabeu@gmail.com
3. Include:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

### What to Expect

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 1 week
- **Resolution Timeline**: Depends on severity, typically 30-90 days

### Safe Harbor

We consider security research conducted in accordance with this policy to be:
- Authorized concerning any applicable anti-hacking laws
- Exempt from restrictions in our Terms of Service that would interfere with conducting security research

We will not pursue civil or criminal action against researchers who:
- Make a good faith effort to avoid privacy violations and disruption
- Do not exploit vulnerabilities beyond what is necessary to confirm them
- Report vulnerabilities promptly

## Security Best Practices for Users

### API Keys

- Never commit API keys to version control
- Use environment variables for sensitive configuration
- Rotate API keys regularly

### Configuration

- The `.grepai/` directory should not be shared
- Add `.grepai/` to your `.gitignore`
- Review `config.yaml` before sharing configurations

### Network Security

- When using Ollama: runs locally, no data leaves your machine
- When using OpenAI: code content is sent to OpenAI's API
- PostgreSQL connections should use SSL in production

## Known Security Considerations

1. **Embedding Content**: Code content is sent to embedding providers
   - Use Ollama for sensitive codebases
   - Review OpenAI's data usage policies before using

2. **Index Files**: The `.grepai/index.gob` contains embeddings of your code
   - Do not share index files
   - Index files are excluded from git by default

3. **PostgreSQL Backend**: When using shared databases
   - Use strong passwords
   - Enable SSL
   - Implement proper access controls

4. **Untrusted Repositories**: Run grepai only on code you trust, as you would `npm install`
   - To compile `.vue` files, grepai runs Node.js with the `@vue/compiler-sfc` package found in the project's own `node_modules`. A repository that ships its own copy of that package can run code on your machine during `index`, `watch` or MCP indexing
   - Disable the Vue processor (`framework_processing.frameworks.vue.enabled: false`) before indexing a repository you do not trust
