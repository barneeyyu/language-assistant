# Language Assistant

This project is an English language assistant LINE bot designed to help users translate words, provide example sentences, synonyms, and antonyms. Additionally, it offers a daily summary of all the words looked up throughout the day.

## Setup

1. Address all `TODO` comments.

2. Install [Node.js](https://nodejs.org/) and [Go](https://go.dev/).

3. Install and upgrade all packages to ensure your application is initialized with the latest package versions.  Note, this only need be done once.

       go get -t -u ./...
       go mod tidy

       npm update
       npx ncu -u

4. Commit changes

       git add .
       git commit

That's it, you are good to start coding!

For more on building AWS Lambdas with Go, see [AWS docs](https://docs.aws.amazon.com/lambda/latest/dg/lambda-golang.html)

## Golang Linter Configuration

Change the .golangci.yml file to match your project's needs.

Execute the linter with:

    make lint

If you want to run the linter with auto-fixing, run:

    make lint-fix

## Build

Run the following command to build the application:

    make clean build

## Test

Testing setup and execution is left to the developer.

## Deploy
### Deploy to AWS on local
Run the following command to deploy the application:

1. Login aws first, paste your aws credentials
```bash
aws configure
#and paste your keys
```

2.
    2.1Copy the environment template file:
    ```
    cp .env.example .env
    ```
    2.2 Edit the `.env` file and fill in your LINE Bot credentials:
    ```bash
    CHANNEL_SECRET={{your_channel_secret}}
    CHANNEL_TOKEN={{your_channel_token}}
    # These credentials can be found in the LINE Developers Console.
    ```

1. Deploy to AWS
```bash
sls deploy --stage {stageName} --verbose
```

1. When deploy successfully, copy the url of `line-events` API, and paste to the ***Line developers*** -> Webhook URL
