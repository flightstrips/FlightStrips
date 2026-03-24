---
title: EuroScope plugin
description: Install the FlightStrips EuroScope plugin on Windows—download, unblock, copy into Plugins, then sign in.
---
## Download

Open the GitHub releases page:

**[github.com/flightstrips/FlightStrips/releases](https://github.com/flightstrips/FlightStrips/releases)**

Pick the **newest** release that is for the **plugin** (the release title says so). Under **Assets**, download only these two files:

- **FlightStripsPlugin.dll**
- **flightstrips_config.ini**

Save them somewhere easy to find (for example your **Downloads** folder). If you get a **.zip** file, unzip it first—you only need these two files from inside.

## Unblock the files

After download, Windows may block the files or warn you about them.

**Unblock (recommended)**

1. Right-click each file → **Properties**.
2. On the **General** tab, if you see **Unblock**, turn it on → **OK**.

Do that for **both** files.

**Windows Defender**

If Windows Defender or SmartScreen stops the download, shows a warning, or removes a file, treat that as normal for downloaded programs. If you trust FlightStrips, use **Keep** / **Run anyway** where Windows offers it. If a file was quarantined, open **Windows Security** → **Virus & threat protection** → **Protection history** and restore or allow the file if appropriate.

## 3. Put the files in EuroScope

1. **Close EuroScope** completely.
2. Copy **both** files into your EuroScope **Plugins** folder for **your** airport package. The folder name is your four-letter airport code (for example **EKCH** for Copenhagen).

   Easiest way to get there:

   - Press **Win + R**, type `%AppData%`, press **Enter**.
   - Open **EuroScope** → your airport folder (for example **EKCH**) → **Plugins**.

3. Leave **FlightStripsPlugin.dll** and **flightstrips_config.ini** **together** in that **Plugins** folder (not in a subfolder).

## 4. Load the plugin

1. Start EuroScope.
2. Use **Plug-ins** → **Load** (the exact wording may differ) and choose **FlightStripsPlugin.dll** from your **Plugins** folder.

## 5. Sign in

1. In EuroScope, run the chat command **`.fs open`** to open the FlightStrips panel.
2. Click **Login** and finish signing in in your browser when it opens.

When you are signed in, the panel shows your account; you can sign out from the same place when you need to.
