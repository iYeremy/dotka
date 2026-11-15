# Publishing to the AUR

`dotka-git/PKGBUILD` builds the latest `main` from GitHub. The AUR has its own git repo
per package, so this folder is just the source of truth; you copy it over to publish.

## First time

1. AUR account with your SSH public key added (My Account -> SSH Public Key).
2. Tell ssh to use that key for the AUR, in `~/.ssh/config`:

   ```
   Host aur.archlinux.org
     IdentityFile ~/.ssh/aur
     User aur
   ```

3. Push the code to GitHub first (the PKGBUILD clones from there), and tag a release so
   the version looks nice (`0.1.0.r0.gabc1234` instead of `r56.abc1234`):

   ```bash
   git push origin main
   git tag -a v0.1.0 -m "dotka 0.1.0"
   git push origin v0.1.0
   ```

4. Build it once from GitHub to be sure, then publish:

   ```bash
   cd /tmp && git clone ssh://aur@aur.archlinux.org/dotka-git.git && cd dotka-git
   cp ~/proyectos/Dotka/packaging/aur/dotka-git/PKGBUILD .
   makepkg -f                              # clones from GitHub, runs tests, builds
   makepkg --printsrcinfo > .SRCINFO       # after makepkg so pkgver is updated
   namcap PKGBUILD *.pkg.tar.zst           # optional lint (pacman -S namcap)
   git add PKGBUILD .SRCINFO
   git commit -m "Initial import: dotka-git 0.1.0"
   git push
   ```

   Don't commit the built packages or `src/` / `pkg/` folders to the AUR repo.

## Updates

A `-git` package doesn't need updating for every commit: `pkgver()` figures out the
version at build time. Only push a new PKGBUILD/.SRCINFO when the build itself changes
(new deps, new files to install...). Bump `pkgrel` when you do, and regenerate
`.SRCINFO`.

## Later: a stable `dotka` package

Once there are tagged releases, a non-git `dotka` package can download
`https://github.com/iYeremy/dotka/archive/v$pkgver.tar.gz` with a real `sha256sums`
(`updpkgsums` fills it in). Same `build()`/`check()`/`package()` as the -git one.
