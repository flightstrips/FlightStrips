namespace Vatsim.Scandinavia.FlightStrips.Host.Hubs.Models;

public class ControllerUpdateModel
{
    public ControllerState State { get; set; }

    public required string Frequency { get; set; }

    public required string Position { get; set; }
}

public enum ControllerState
{
    Online,
    Offline
}
