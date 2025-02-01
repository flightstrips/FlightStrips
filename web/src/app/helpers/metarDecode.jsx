'use strict';
import metarParser from 'aewx-metar-parser'
const metarObject = metarParser('KSFO 070121Z 19023KT 1 1/2SM R28R/6000VP6000FT -RA BKN004 BKN013 OVC035 15/12 A2970 RMK AO2 T01500122 PNO $');
console.log(metarObject);