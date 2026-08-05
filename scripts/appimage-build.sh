#!/bin/bash -e
if (ls supersonic);
then
rm -rf Supersonic.AppDir/
mkdir -p Supersonic.AppDir/usr/lib
mkdir -p Supersonic.AppDir/usr/bin

if [[ "$@" != *"--manual-libs"* ]]; then
  # auto add all dependences of binary except those on AppImage exclude list
  EXCLUDE=$(wget -nv -O - https://github.com/AppImageCommunity/pkg2appimage/raw/refs/heads/master/excludelist | grep -v "^#")
  ldd ./supersonic | grep -F "x86_64-linux-gnu" | sort | while read -r file _ path __; do 
    if [[ "$EXCLUDE" == *"$file"* ]]; then
      echo "exclude $file"
    else
      echo "add $path"
      cp $path "Supersonic.AppDir/usr/lib/$file"
    fi
  done
else
  # (smaller) manual list of dependencies which avoids some libs that are generally available on most linux installs already
  set -x
  cp /usr/lib/x86_64-linux-gnu/libpostproc.so.55 Supersonic.AppDir/usr/lib #
  cp /usr/lib/x86_64-linux-gnu/libsrt-gnutls.so.1.4 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libx264.so.163 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libcodec2.so.1.0 Supersonic.AppDir/usr/lib/
  cp /usr/lib/x86_64-linux-gnu/libdav1d.so.5 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libvpx.so.7 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libmfx.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libavdevice.so.58 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libswresample.so.3 Supersonic.AppDir/usr/lib/ 
  cp /usr/lib/x86_64-linux-gnu/libavfilter.so.7 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libswscale.so.5 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libavformat.so.58 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libavutil.so.56 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libavcodec.so.58 Supersonic.AppDir/usr/lib/ 
  cp /usr/lib/x86_64-linux-gnu/libuchardet.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libsixel.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/liblua5.2.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libplacebo.so.192 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libmujs.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libcdio_cdda.so.2 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libcdio_paranoia.so.2 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libmpv.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libjpeg.so.8 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libass.so.9 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libdvdnav.so.4 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libbluray.so.2 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/librubberband.so.2 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libzimg.so.2 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libjack.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libva-wayland.so.2 Supersonic.AppDir/usr/lib/ #Don't delete
  cp /usr/lib/x86_64-linux-gnu/libzvbi.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libsnappy.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libgsm.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libshine.so.3 Supersonic.AppDir/usr/lib/ #
  #cp /usr/lib/x86_64-linux-gnu/libSvtAv1Enc.so.1 Supersonic.AppDir/usr/lib/ #not required?
  cp /usr/lib/x86_64-linux-gnu/libx265.so.199 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libxvidcore.so.4 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libpocketsphinx.so.3 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libbs2b.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/liblilv-0.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libmysofa.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libflite_cmu_us_awb.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libvidstab.so.1.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libzmq.so.5 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libgme.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libopenmpt.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libchromaprint.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/librabbitmq.so.4 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libsoxr.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libopenal.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libdc1394.so.25 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libva.so.2 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libsphinxbase.so.3 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libflite_cmu_us_kal.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libflite_cmu_us_rms.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libflite_cmu_us_slt.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libdvdread.so.8 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libudfread.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libserd-0.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libsord-0.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libsratom-0.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libpgm-5.3.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libnorm.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libsndio.so.7 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libssh-gcrypt.so.4 Supersonic.AppDir/usr/lib/ # Don't delete
  cp /usr/lib/x86_64-linux-gnu/libflite_cmu_us_kal16.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libblas.so.3 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/liblapack.so.3 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libflite.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libflite_usenglish.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libflite_cmulex.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libgfortran.so.5 Supersonic.AppDir/usr/lib/ #Don't delete
  cp /usr/lib/x86_64-linux-gnu/libbz2.so.1.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libdb-5.3.so Supersonic.AppDir/usr/lib/ #
  #cp /usr/lib/x86_64-linux-gnu/libstdc++.so.6 Supersonic.AppDir/usr/lib/ # not required (Maybe?)
  cp /usr/lib/x86_64-linux-gnu/libsodium.so.23 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libcaca.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libXss.so.1 Supersonic.AppDir/usr/lib/ # Fedora required
  cp /usr/lib/x86_64-linux-gnu/libncursesw.so.6 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libtinfo.so.6 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libnuma.so.1 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libavc1394.so.0 Supersonic.AppDir/usr/lib/ #
  cp /usr/lib/x86_64-linux-gnu/libquadmath.so.0 Supersonic.AppDir/usr/lib #
  cp /usr/lib/x86_64-linux-gnu/libOpenCL.so.1 Supersonic.AppDir/usr/lib/ # Opensuse tumbleweed fix
  cp /usr/lib/x86_64-linux-gnu/libtheoraenc.so.1 Supersonic.AppDir/usr/lib/ # Opensuse tumbleweed fix
  cp /usr/lib/x86_64-linux-gnu/libtheoradec.so.1 Supersonic.AppDir/usr/lib/ # Opensuse tumbleweed fix
  cp /usr/lib/x86_64-linux-gnu/libxml2.so.2 Supersonic.AppDir/usr/lib/ # CachyOS fix
  cp /usr/lib/x86_64-linux-gnu/libicuuc.so.70 Supersonic.AppDir/usr/lib/ # CachyOS fix
  cp /usr/lib/x86_64-linux-gnu/libicudata.so.70 Supersonic.AppDir/usr/lib/ # CachyOS fix
  set +x
fi

printf '%s\n' '#!/bin/bash' 'SELF=$(readlink -f "$0")' 'HERE=${SELF%/*}' 'EXEC="${HERE}/usr/bin/supersonic"' 'export LD_LIBRARY_PATH="/usr/lib64:/lib64:/usr/lib/x86_64-linux-gnu:/usr/lib:${HERE}/usr/lib/"' 'exec "${EXEC}";' > Supersonic.AppDir/AppRun
printf '%s\n' '[Desktop Entry]' 'Name=Supersonic' 'Exec=supersonic' 'Icon=ico' 'Type=Application' 'Comment=A lightweight cross-platform desktop client for self-hosted music servers' 'Categories=AudioVideo;' > Supersonic.AppDir/"supersonic.desktop"
chmod +x Supersonic.AppDir/AppRun
chmod +x Supersonic.AppDir/supersonic.desktop
wget -nv https://raw.githubusercontent.com/supersonic-app/supersonic/main/res/appicon.png -O Supersonic.AppDir/ico.png
cp supersonic Supersonic.AppDir/usr/bin/
chmod +x Supersonic.AppDir/usr/bin/supersonic

wget -nv -nc https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage
chmod +x appimagetool-x86_64.AppImage
./appimagetool-x86_64.AppImage Supersonic.AppDir/

echo "Script finished"
else
echo "executable not found!"
fi
