# FlightStrips plugin for EuroScope

This is the FlightStrips for the EuroScope which handles all the communication with the FlightStrips API.

The on-screen FlightStrips panel and departure-clearance popup are automatically kept inside the current radar area. If the saved panel position still needs to be recovered, enter `.fs reset` in EuroScope's command line to move it back near the top-left corner and reopen it.

Use the panel's **GRAPHIC** button to open a compact stand-occupancy timeline for the current airport and session. It shows a four-hour window centered on now, refreshes every 15 seconds, and can be refreshed or moved from its title bar.

## Credit

Many parts of the plugin is inspired by the [UK Controller Plugin](https://github.com/VATSIM-UK/uk-controller-plugin)
