# s1h: ssh & scp in a unified TUI

s1h is a simple TUI inspired by [K9s](https://github.com/derailed/k9s).
This repository contains two command-line tools written in Golang:

1. **s1h** - Quickly SSH/SCP into configured hosts defined in your ssh config file.
2. **s1hpass** - Manage credentials securely. Used for key-less access.

## Installation

```sh
# Install from Go directly:
go install github.com/noboruma/s1h/cmd/s1h@latest
go install github.com/noboruma/s1h/cmd/s1hpass@latest

# Or build from the repository source
git clone https://github.com/noboruma/s1h
cd s1h
make build

# Or install directly using Go
go install ./...

# Or download the binaries from the release
wget https://github.com/noboruma/releases
```

## Tools Overview

### s1h

The `s1h` tool reads the SSH config file and allows you to select a host to SSH into using either a password or SSH keys.

#### Usage:

```sh
s1h
```
This command displays a list of available SSH hosts from your `~/.ssh/config`, allowing you to select one and connect. It also allows you to use scp commands.

#### Example:

Let's image you have the following SSH config file (i.e.` ~/.ssh/config`):
```
Host alive-vm
Hostname *.*,*,*
User root
IdentiftyFile ~/my-priv-key

Host dead-vm
Hostname *.*,*,*
User root
IdentiftyFile ~/my-priv-key

Host alive-vm2
Hostname *.*,*,*
User root
```
Simple execute the following:
```
s1h
```
![main output](.github/assets/main.png)

<span style="color:green">Green entries</span> are ssh reachable hosts. <span style="color:green">Red</span> indicates the host are not reachable with the given hostname & port.
You can search hosts or hostname using repectively `F1` amd `F4` to jump directly to entries:

![main output](.github/assets/search.png)

- If you press `enter` and it will automatically use the configured authentication method (password or SSH key) to establish the connection. This opens a new shell on the remote host.

- If you press `c` it will give the option to upload a file to the selected host:
![main output](.github/assets/upload.png)

- If you press `C` it will give the option to download a file from the selected host:
![main output](.github/assets/download.png)

### s1hpass

The `s1hpass` tool provides options to create an encryption key and update username-password pairs securely. This is useful for host that requires password instead of a key.

#### Usage:

```sh
s1hpass create-key
```
This command generates a new encryption key for securing credentials stored locally.

```sh
s1hpass upsert <host> <password>
```
This updates the stored credentials for the specified ssh host.

#### Example:

```sh
s1hpass create-key
# Output: Master key saved to ~/.config/s1h/master.key

s1hpass upsert remote-vm mySecureP@ss
# Output: Credentials updated.

s1hpass remove remote-vm
# Output: Credentials removed.
```

---

## License

This project is licensed under the MIT License.

## Contributing

Pull requests are welcome! Feel free to submit issues or suggestions.

---

**Author:** Thomas Legris

