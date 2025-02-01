# s1h: k9s-inspired ssh layer

This repository contains two command-line tools written in Golang:

1. **passwd** - Manage credentials securely.
2. **s1h** - Quickly SSH into configured hosts defined in your ssh config file.

## Installation

```sh
# Clone the repository
git clone https://github.com/noboruma/s1h
cd s1h

# Build the tools
make build

# Or install directly using Go
go install ./...
```

## Tools Overview

### passwd

The `passwd` tool provides options to create an encryption key and update username-password pairs securely. This is useful for host that requires password instead of a key.

#### Usage:

```sh
passwd create-key
```
This command generates a new encryption key for securing credentials stored locally.

```sh
passwd upsert <host> <password>
```
This updates the stored credentials for the specified ssh host.

#### Example:

```sh
passwd create-key
# Output: Key created successfully!

passwd update remotevm1 mySecureP@ss
# Output: Credentials updated for remotevm1.
```

---

### s1h

The `s1h` tool reads the SSH config file and allows you to select a host to SSH into using either a password or SSH keys.

#### Usage:

```sh
s1h
```
This command displays a list of available SSH hosts from your `~/.ssh/config`, allowing you to select one and connect.

#### Example:

```sh
s1h
# Output:

#Host (F1)   User (F2) Port (F3) HostName (F4)    IdentityFile…  Password
#server1     root      22        1.2.3.4          ~/.ssh/id_ed…
#server2     root      22        4.5.6.7          ~/.ssh/id_ed…
#server3     root      22        7.8.9.0                         O

```

You can search using associated F1/F2/F3/F4 to jump directly to the entry.
Press enter and it will automatically use the configured authentication method (password or SSH key) to establish the connection.

## License

This project is licensed under the MIT License.

## Contributing

Pull requests are welcome! Feel free to submit issues or suggestions.

---

**Author:** Thomas Legris
**Repository:** [GitHub](https://github.com/noboruma/s1h)

