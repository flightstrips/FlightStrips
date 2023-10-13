using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

internal class UpsertStripRequestModel
{
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public StripState State { get; set; } = StripState.None;
    public bool Cleared { get; set; }
}