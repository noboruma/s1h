# s1h

ssh + scp + port forwarding + password manager in one terminal UI, inspired by [K9s](https://github.com/derailed/k9s).
It reads your `~/.ssh/config` and works on one or several hosts at once, with SSH keys or locally encrypted passwords.

```sh
go install github.com/noboruma/s1h/cmd/s1h@latest
```

or

```sh
git clone https://github.com/noboruma/s1h && cd s1h && go install ./...
```

or

```sh
# pick one of: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64
target=linux-amd64
curl -LO "https://github.com/noboruma/s1h/releases/latest/download/s1h-${target}"
curl -LO "https://github.com/noboruma/s1h/releases/latest/download/s1h-${target}.sha256"
sha256sum -c "s1h-${target}.sha256"   # on macOS: shasum -a 256 -c
chmod +x "s1h-${target}" && mv "s1h-${target}" ~/.local/bin/s1h
```

or download the binaries in the [Github release section](https://github.com/noboruma/s1h/releases/latest).
macOS binaries are not signed: if macOS blocks one, run `xattr -d com.apple.quarantine s1h`.

![main view](.github/assets/main.png)

## Features

| Feature | Keys | Notes |
| --- | --- | --- |
| Reachability | | ✅/❌ per host, checked every 2 minutes |
| Shell | `[s]` | Uses the stored password or SSH keys |
| Upload | `[u]` | To the selected host, or to all marked hosts |
| Download | `[d]` | From the selected or marked hosts; `*` in the destination is replaced by each host name |
| Exec | `[x]` | Short-lived command on the selected or marked hosts |
| Mark | `[space]` `[M]` | Toggle one host / mark all reachable (or clear) for multi-host actions |
| Filter | `[/]` | By host or hostname; `[enter]` keeps it, `[esc]` clears it |
| Add / edit / remove host | `[+]` `[e]` `[-]` | Written to its `Host` block in `~/.ssh/config` (other lines are kept, new hosts go before `Host *`); only the password goes to `s1h` |
| Reveal password | `[r]` | Shows the selected host's stored password |
| Config port forwards | `[e]` | `LocalForward` lines, in `ssh -L` syntax: `8080:localhost:80, 0.0.0.0:9000:db:9000` or `9000`. Always on |
| `s1h` port forwards | `[p]` | Kept in `s1h`, never in `~/.ssh/config`. In the popup: `[enter]` open/close, `[space]` enable/disable, `[x]` forget |
| Open port forwards | `[R]` `[F]` | Opens the selected host's / every host's config forwards and enabled `s1h` forwards. They stay open until closed or `s1h` exits; *Forwards* column: `●` open, `○` closed, `⊘` disabled |
| CLI | | `s1h shell remote1`, `s1h cp remote1:/path/file remote2:/path/` |
| Quit | `[q]` `[esc]` | `[esc]` closes a popup or clears the filter first |

## Passwords

Passwords are stored in `~/.config/s1h/credentials.enc`, encrypted with AES-256-GCM using a random key in
`~/.config/s1h/master.key`, both mode `0600`.

> [!WARNING]
> Anyone who can read both files can decrypt the passwords. Keep `master.key` on a USB stick and point
> `S1H_MASTER_KEY` at it: passwords are then only usable while the stick is mounted. The header shows
> `Master key: separate disk` or `⚠ same disk as credentials`. Back the key up separately: without it,
> `credentials.enc` can't be recovered. Prefer SSH keys where you can.

```sh
mv ~/.config/s1h/master.key /media/usb/s1h-master.key
export S1H_MASTER_KEY=/media/usb/s1h-master.key   # in your shell profile
```

## License

MIT. Pull requests and issues are welcome.
