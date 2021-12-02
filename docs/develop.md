# Building

## ntfy server
To quickly build on amd64, you can use `make build-simple`:

```
git clone git@github.com:binwiederhier/ntfy.git
cd ntfy
make build-simple
```

That'll generate a statically linked binary in `dist/ntfy_linux_amd64/ntfy`.

For all other platforms (including Docker), and for production or other snapshot builds, you should use the amazingly 
awesome [GoReleaser](https://goreleaser.com/) make targets:

```
Build:
  make build                       - Build
  make build-snapshot              - Build snapshot
  make build-simple                - Build (using go build, without goreleaser)
  make clean                       - Clean build folder

Releasing (requires goreleaser):
  make release                     - Create a release
  make release-snapshot            - Create a test release
```

There are currently no platform-specific make targets, so they will build for all platforms (which may take a while).

## Android app
The Android app has two flavors:

* **Google Play:** The `play` flavor includes Firebase (FCM) and requires a Firebase account
* **F-Droid:** The `fdroid` flavor does not include Firebase or Google dependencies

First check out the repository:

```
git clone git@github.com:binwiederhier/ntfy-android.git
cd ntfy-android
```

Then either follow the steps for building with or without Firebase.

### Building without Firebase (F-Droid flavor)
Without Firebase, you may want to still change the default `app_base_url` in [strings.xml](https://github.com/binwiederhier/ntfy-android/blob/main/app/src/main/res/values/strings.xml)
if you're self-hosting the server. Then run:
```
# To build an unsigned .apk (app/build/outputs/apk/fdroid/*.apk)
./gradlew assembleFdroidRelease

# To build a bundle .aab (app/fdroid/release/*.aab)
./gradlew bundleFdroidRelease
```

### Building with Firebase (FCM, Google Play flavor)
To build your own version with Firebase, you must:
* Create a Firebase/FCM account
* Place your account file at `app/google-services.json`
* And change `app_base_url` in [strings.xml](https://github.com/binwiederhier/ntfy-android/blob/main/app/src/main/res/values/strings.xml)
* Then run:
```
# To build an unsigned .apk (app/build/outputs/apk/play/*.apk)
./gradlew assemblePlayRelease

# To build a bundle .aab (app/play/release/*.aab)
./gradlew bundlePlayRelease
```
