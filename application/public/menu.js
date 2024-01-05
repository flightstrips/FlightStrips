const { app, Menu } = require("electron");

const isMac = process.platform === "darwin";

const template = [
  {
    label: "DEV",
    submenu: [
      { role: "toggleDevTools" },
    ],
  },
  {
    label: "Views",
    submenu: [
      {
        label: "EKCH DEL",
        click() {
          menu.webContents.send("navigate", "/");
        },
      },
      { type: "separator" },
    ],
  },
  {
    label: "Window",
    submenu: [
      { role: "forceReload" },
      { role: "togglefullscreen" },
    ],
  },
  {
    label: "Misc",
    submenu: [{ label: "VATCAN Event Code" }, { type: "separator" }],
  },

  {
    role: "help",

    submenu: [
      {
        label: "Documentation",
        click: async () => {
          const { shell } = require("electron");
          await shell.openExternal("https://docs.fstools.dk");
        },
      },
      { type: "separator" },
      {
        label: "Discord",
        click: async () => {
          const { shell } = require("electron");
          await shell.openExternal("https://discord.gg/vatsca");
        },
      },
      {
        label: "Github Issues",
        click: async () => {
          const { shell } = require("electron");
          await shell.openExternal(
            "https://github.com/frederikrosenberg/FlightStrips/issues"
          );
        },
      },
    ],
  },
];

const menu = Menu.buildFromTemplate(template);
Menu.setApplicationMenu(menu);
