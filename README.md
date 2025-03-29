# Language Assistant

<img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
<img src="https://img.shields.io/badge/Amazon_AWS-FF9900?style=for-the-badge&logo=amazonaws&logoColor=white" />
<img src="https://img.shields.io/badge/Amazon%20DynamoDB-4053D6?style=for-the-badge&logo=Amazon%20DynamoDB&logoColor=white" />
<img src="https://img.shields.io/badge/Line-00C300?style=for-the-badge&logo=line&logoColor=white" />
<img src="https://img.shields.io/badge/ChatGPT-74aa9c?style=for-the-badge&logo=openai&logoColor=white" />

![Serverless](https://img.shields.io/badge/Serverless-FD5750?style=flat-square&logo=serverless&logoColor=white)

![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/barneeyyu/language-assistant?style=for-the-badge)
![GitHub last commit](https://img.shields.io/github/last-commit/barneeyyu/language-assistant?style=for-the-badge)
![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge&logo=license)


A LINE bot that serves as your English language companion, offering:
- Word translation
- Example sentences
- Synonyms and antonyms
- Daily summary of looked-up words

## Prerequisites
- [Node.js](https://nodejs.org/)
- [Go](https://go.dev/)
- AWS Account and AWS CLI
- LINE Developer Account

## Quick Start

1. Clone the repository
```bash
git clone [repository-url]
cd language-assistant
```

2. Install dependencies
```bash
# Update Go dependencies
go get -t -u ./...
go mod tidy

# Update Node.js dependencies
npm update
npx ncu -u
```

3. Configure LINE Bot
- Copy the environment template:
```bash
cp .env.example .env
```
- Edit `.env` with your LINE Bot credentials from LINE Developers Console:
```bash
CHANNEL_SECRET=your_channel_secret
CHANNEL_TOKEN=your_channel_token
```

4. Deploy to AWS
```bash
# Configure AWS credentials
aws configure

# Deploy using Serverless Framework
sls deploy --stage prod --verbose
```

5. Set Webhook URL
- Copy the generated `line-events` API URL
- Set it as Webhook URL in LINE Developers Console

## Development

### Linting
```bash
# Run linter
make lint

# Run linter with auto-fix
make lint-fix
```

### Building
```bash
make clean build
```

## Documentation
- [AWS Lambda Golang Guide](https://docs.aws.amazon.com/lambda/latest/dg/lambda-golang.html)
- [LINE Messaging API](https://developers.line.biz/en/docs/messaging-api/)
