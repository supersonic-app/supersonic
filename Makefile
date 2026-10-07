build:
	go build -tags migrated_fynedo

# dylibbundler doesn't seem to pick up on the Python framework dependency,
# so the last 3 cmds move it over manually. This is a bit fragile though
# since it assumes a specific location and version of the dependency
package_macos:
	fyne package -os darwin -tags migrated_fynedo

bundledeps_macos_homebrew:
	dylibbundler -od -b -x ./Supersonic.app/Contents/MacOS/supersonic -d ./Supersonic.app/Contents/Frameworks/ -p @executable_path/../Frameworks/
	./scripts/copy_python_dep_osx.sh
	codesign --force --deep --preserve-metadata=entitlements,requirements,flags,runtime --sign - "./Supersonic.app/Contents/MacOS/supersonic"

bundledeps_macos_macports:
	dylibbundler -od -b -x ./Supersonic.app/Contents/MacOS/supersonic -d ./Supersonic.app/Contents/Frameworks/ -p @executable_path/../Frameworks/
	codesign --force --deep --preserve-metadata=entitlements,requirements,flags,runtime --sign - "./Supersonic.app/Contents/MacOS/supersonic"

bundledeps_macos_highsierra:
	mkdir ./Supersonic.app/Contents/Frameworks
	cp -R res/libs/mac_x64/mpv/* ./Supersonic.app/Contents/Frameworks
	install_name_tool -change "/usr/local/opt/mpv/lib/libmpv.2.dylib" "@executable_path/../Frameworks/libmpv.2.dylib" "./Supersonic.app/Contents/MacOS/supersonic"

zip_macos:
	zip --symlinks -r Supersonic.zip Supersonic.app/

package_windows:
	fyne package -os windows -tags migrated_fynedo

package_windows_arm64:
	CC=clang CXX=clang++ fyne package -os windows -tags migrated_fynedo

package_linux:
	fyne package -os linux -tags migrated_fynedo

.PHONY: lint
lint:
	golangci-lint run
