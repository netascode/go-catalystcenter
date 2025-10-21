## 0.1.9

- Retry on authentication requests

## 0.1.8

- Remove trailing slash from base URL to prevent double slashes in the final request URL

## 0.1.7

- Add `writingMutex` to prevent multiple concurrent operations (such as Create, Update, Delete) and introduce `UseMutex` option in `Request` to enable it

## 0.1.6

- Honor proxy settings (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` environment variables)
- Add `NoWait` option to `Request` to disable waiting for async operations

## 0.1.5

- Handle pagination of large GET responses transparently

## 0.1.4

- Implement synchronous request only for non-GET requests

## 0.1.3

- Optimize wait time after 429 response

## 0.1.2

- Retry on 429 HTTP responses

## 0.1.1

- Add option to set default maximum waiting time for async operations and override ip per request

## 0.1.0

- Initial release
