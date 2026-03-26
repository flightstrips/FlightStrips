
#pragma once

#define AIRPORT "EKCH"

#define COMMAND_PREFIX ".fs"
#define COMMAND_OPEN COMMAND_PREFIX " open"
#define COMMAND_CLOSE COMMAND_PREFIX " close"
#define COMMAND_CDM_MASTER COMMAND_PREFIX " cdm master"
#define COMMAND_CDM_SLAVE COMMAND_PREFIX " cdm slave"

constexpr int TAG_ITEM_DEICING_DESIGNATOR = 1;
constexpr int TAG_ITEM_CDM_TOBT = 2;
constexpr int TAG_ITEM_CDM_REQ_TOBT = 3;
constexpr int TAG_ITEM_CDM_TSAT = 4;
constexpr int TAG_ITEM_CDM_TTOT = 5;
constexpr int TAG_ITEM_CDM_CTOT = 6;
constexpr int TAG_ITEM_CDM_STATUS = 7;
constexpr int TAG_ITEM_CDM_ASRT = 8;
constexpr int TAG_ITEM_CDM_TSAC = 9;
constexpr int TAG_ITEM_CDM_EOBT = 101;
constexpr int TAG_ITEM_CDM_TSAT_TOBT_DIFF = 102;
constexpr int TAG_ITEM_CDM_FLOW_MESSAGE = 103;
constexpr int TAG_ITEM_CDM_NETWORK_STATUS = 104;
constexpr int TAG_ITEM_CDM_TOBT_CONFIRMED_BY = 105;
constexpr int TAG_ITEM_CDM_PHASE = 106;
constexpr int TAG_ITEM_CDM_TTG = 107;
constexpr int TAG_ITEM_CDM_READY_STARTUP = 108;
constexpr int TAG_ITEM_CDM_ASAT = 109;

constexpr int TAG_FUNC_CDM_EDIT_TOBT = 2002;
constexpr int TAG_FUNC_CDM_SET_TOBT = 2003;
constexpr int TAG_FUNC_CDM_TOGGLE_ASRT = 2004;
constexpr int TAG_FUNC_CDM_EDIT_DEICE = 2005;
constexpr int TAG_FUNC_CDM_SET_DEICE = 2006;
constexpr int TAG_FUNC_CDM_EDIT_MANUAL_CTOT = 2007;
constexpr int TAG_FUNC_CDM_SET_MANUAL_CTOT = 2008;
constexpr int TAG_FUNC_CDM_REMOVE_MANUAL_CTOT = 2009;
constexpr int TAG_FUNC_CDM_APPROVE_REQ_TOBT = 2010;
constexpr int TAG_FUNC_CDM_EOBT_ACTION = 2011;
constexpr int TAG_FUNC_CDM_EOBT_TO_TOBT = 2012;
constexpr int TAG_FUNC_CDM_READY_TOBT = 2013;
constexpr int TAG_FUNC_CDM_TOBT_OPTIONS = 2014;
constexpr int TAG_FUNC_CDM_CTOT_OPTIONS = 2015;
constexpr int TAG_FUNC_CDM_OPTIONS = 2016;
constexpr int TAG_FUNC_CDM_FLOW_MESSAGE_AS_TEXT = 2017;
constexpr int TAG_FUNC_CDM_NETWORK_STATUS_OPTIONS = 2018;
constexpr int TAG_FUNC_CDM_DEICE_OPTIONS = 2019;
constexpr int TAG_FUNC_CDM_CLEAR_DEICE = 2020;
constexpr int TAG_FUNC_CDM_SET_DEICE_LIGHT = 2021;
constexpr int TAG_FUNC_CDM_SET_DEICE_MEDIUM = 2022;
constexpr int TAG_FUNC_CDM_SET_DEICE_HEAVY = 2023;
constexpr int TAG_FUNC_CDM_SET_DEICE_JUMBO = 2024;
constexpr int TAG_FUNC_CDM_TSAC_OPTIONS = 2025;
constexpr int TAG_FUNC_CDM_TOGGLE_TSAC = 2026;
constexpr int TAG_FUNC_CDM_EDIT_TSAC = 2027;
constexpr int TAG_FUNC_CDM_SET_TSAC = 2028;
