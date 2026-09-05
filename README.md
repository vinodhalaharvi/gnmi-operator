# nRF52840 MDK USB Dongle + ESPHome

Working dir: `~/embedded_projects/makerdiary`

## Files

| File | What it is |
|---|---|
| `mdk-dongle.yaml` | ESPHome config. Blink + internal temperature, nothing else yet. |
| `flash.sh` | Compiles and copies the UF2 onto the bootloader volume. |

## Setup

```bash
cd ~/embedded_projects/makerdiary
cp ~/Downloads/mdk-dongle.yaml ~/Downloads/flash.sh ~/Downloads/README.md .
chmod +x flash.sh

esphome version     # need 2025.8 or newer for the nrf52 platform
./flash.sh mdk-dongle.yaml
```

First compile pulls the whole nRF Connect SDK / Zephyr toolchain. It takes a
while and a few GB. It is not hung.

## First flash

The board is not running ESPHome yet, so it can't reboot itself into DFU:

1. Unplug the dongle
2. Hold the button
3. Plug it in, release the button
4. RGB LED turns green, it mounts as `UF2BOOT` (older units: `MDK-DONGLE`)

`flash.sh` waits for that volume and copies the file. After that, `nrf52.dfu`
is enabled in the config, so plain `esphome upload mdk-dongle.yaml` handles the
reset itself and you don't need the button dance again.

## What's currently on your dongle

Not a blank board. The CDC port you saw (`/dev/cu.usbmodemFDEE160FA5AD2`) is
stock firmware — dongles made after 20 July 2023 ship with Nordic's BLE
Connectivity firmware for the nRF Connect BLE desktop app; earlier ones shipped
with OpenThread NCP. You're only replacing the application, not the bootloader,
so this is reversible (see Recovery below).

## Pin map

Only the RGB LED and one button are on this board.

| Function | Pin | Notes |
|---|---|---|
| LED green | P0.22 | active low |
| LED red | P0.23 | active low |
| LED blue | P0.24 | active low |
| Button | P0.18 | often configured as hardware RESET |

The button is the annoying one. On many of these units `PSELRESET` in UICR is
set so P0.18 acts as reset — pressing it reboots the chip and no GPIO binary
sensor will ever fire. That's why it's commented out in the YAML. If you need
the button as a real input you have to erase PSELRESET, which needs SWD or a
dedicated pselreset-erase firmware.

## If it doesn't boot

Work through these in order:

1. **Flashes fine but never enumerates.** Bootloader memory map mismatch. Change
   `bootloader: adafruit` to `bootloader: adafruit_nrf52_sd132` and reflash. The
   stock connectivity firmware ships with the S132 SoftDevice, so some units'
   bootloaders expect that offset.
2. **Dead on arrival after a config change.** Set `dcdc: false` (already the
   default here). Enabling DC/DC without a correct external LC filter will stop
   the board booting.
3. **Logs look garbled.** Add `libc_nano: false` under `nrf52.framework` —
   newlib-nano mishandles some printf specifiers.
4. **Flashing is flaky.** Update the UF2 bootloader. Makerdiary ships
   self-updating `update-uf2_bootloader-nrf52840_mdk_usb_dongle-<ver>-nosd.uf2`
   files; check the version in `INFO_UF2.TXT` on the mounted volume first.

The bootloader lives below the application and a bad app can't brick it, so you
can always get back to UF2BOOT with the hold-button-and-plug sequence.

## Recovery — back to stock

Grab `connectivity_4.1.4_usb_with_s132_5.1.0.uf2` from the Makerdiary wiki,
enter bootloader mode, drag it onto the volume. You're back to the BLE
Connectivity firmware.

## What ESPHome can and can't do here

Can: BLE peripheral (`zephyr_ble_server`, `ble_nus`), OpenThread, Zigbee end
device, GPIO, ADC, PWM, internal temperature, deep sleep, OTA via mcumgr.

Can't: **Bluetooth proxy** — that component is ESP32-only. And there's no WiFi
on nRF52840, so no native API or web server unless you route it over Thread with
a border router.

Worth being honest about the fit: ESPHome's nRF52 support is aimed at low-power
battery sensor nodes. This dongle is USB-powered with no sensors on it. Blink
and internal temperature is genuinely all you can do without soldering to the
castellated pads. If what you actually wanted was a BLE proxy or a Zigbee
coordinator for Home Assistant, the stock/vendor firmware paths are the better
tool and ESPHome is the wrong one.

Support for the nRF52 platform is still described upstream as in development —
expect rough edges.
