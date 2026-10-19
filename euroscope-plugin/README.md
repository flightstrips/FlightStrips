# FlightStrips plugin for EuroScope

This is the FlightStrips for the EuroScope which handles all the communication with the FlightStrips API.

The on-screen FlightStrips panel and departure-clearance popup are automatically kept inside the current radar area. If the saved panel position still needs to be recovered, enter `.fs reset` in EuroScope's command line to move it back near the top-left corner and reopen it.

The expanded panel always shows `Master: <callsign>` below the connection status, even when Stats is collapsed. This identifies the controller whose EuroScope runway configuration governs the session. It shows `Master: None` when connected without a known master, and `Master: Unknown` while disconnected or reconnecting rather than displaying a stale callsign.

## Credit

Many parts of the plugin is inspired by the [UK Controller Plugin](https://github.com/VATSIM-UK/uk-controller-plugin)

