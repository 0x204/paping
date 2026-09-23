# paping

paping measures TCP connection times. It connects to a port again and again,
the way `ping` sends echo requests, and shows how long each connection takes to
establish and which ISP the address belongs to.

![paping output](https://github.com/user-attachments/assets/c2735423-9485-4997-82a2-3ba0fbd13f4d)

## Installation

With Go 1.25 or later:

```sh
go install github.com/0x204/paping@latest
```

Or build from source:

```sh
git clone https://github.com/0x204/paping
cd paping
go build
```

This produces `paping`, or `paping.exe` on Windows.

## Usage

```
paping [options] <host> <port>
```

| Option        | Description                                  | Default               |
| ------------- | -------------------------------------------- | --------------------- |
| `-c count`    | Stop after `count` probes                    | Run until interrupted |
| `-i interval` | Time between probes, such as `1s` or `200ms` | `550ms`               |
| `-t timeout`  | How long to wait for each connection         | `5s`                  |

Without `-c`, paping runs until you press Ctrl+C. Either way, it ends with a
summary:

```console
$ paping -c 4 1.1.1.1 443
Connecting to 1.1.1.1 on TCP 443:

Connected to 1.1.1.1 time=12.42ms protocol=TCP port=443 ISP=AS13335 Cloudflare, Inc.
Connected to 1.1.1.1 time=11.87ms protocol=TCP port=443 ISP=AS13335 Cloudflare, Inc.
Connected to 1.1.1.1 time=12.05ms protocol=TCP port=443 ISP=AS13335 Cloudflare, Inc.
Connected to 1.1.1.1 time=11.93ms protocol=TCP port=443 ISP=AS13335 Cloudflare, Inc.

Connection statistics:
    Attempted = 4, Connected = 4, Failed = 0 (0.00%)
Approximate connection times:
    Minimum = 11.87ms, Maximum = 12.42ms, Average = 12.07ms
```

A hostname with both IPv4 and IPv6 addresses is probed over IPv4.

The exit status is 0 if at least one connection succeeded, 1 if none did, and 2
if the command line is invalid or the host cannot be resolved.

Output is not colored when it goes to a file or pipe, or when `NO_COLOR` is set.

## ISP lookup

The ISP comes from [ipinfo.io](https://ipinfo.io). paping asks for it after the
first successful connection, so the address of a host that never answers is not
sent there, and never asks about loopback, private or link-local addresses. The
ISP shows as `Unknown` until the answer arrives, and stays that way if ipinfo.io
cannot be reached or has no record of the address.

## Development

```sh
go test ./...
golangci-lint run
```
