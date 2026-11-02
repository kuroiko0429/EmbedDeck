#pragma once
// LILYGO T-Embed CC1101, not the original T-Embed.
// Based on vendor Setup214_LilyGo_T_Embed_PN532.h.
#define ST7789_DRIVER
#define TFT_WIDTH 170
#define TFT_HEIGHT 320
#define TFT_INVERSION_ON
#define USE_HSPI_PORT
#define TFT_BL 21
#define TFT_BACKLIGHT_ON HIGH
#define TFT_MISO 10
#define TFT_MOSI 9
#define TFT_SCLK 11
#define TFT_CS 41
#define TFT_DC 16
#define TFT_RST -1
#define LOAD_GLCD
#define LOAD_FONT2
#define SPI_FREQUENCY 40000000
#define SPI_READ_FREQUENCY 20000000
