# Vision One Container Security serverless application template

If you find issues or improvements, please contribute back to the template repo 🙏

_**Rewrite this README for your own application after initial setup**_

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


### Deploy to Production

Deployment to network-one-production is done by GitHub Actions on a push to the main branch

Ensure that the [.github/workflows/release.yaml](.github/workflows/release.yaml) has the TODO addressed to enable deployment to production
