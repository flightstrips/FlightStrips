using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;
using Vatsim.Scandinavia.FlightStrips.Host.Attributes;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class UpsertStripRequestModel
{
    /// <summary>
    /// Origin
    /// </summary>
    /// <example>EKCH</example>
    [Airport]
    public string? Origin { get; set; }

    /// <summary>
    /// Destination
    /// </summary>
    /// <example>EKCH</example>
    [Airport]
    public string? Destination { get; set; }
    public StripState State { get; set; } = StripState.None;
    public bool Cleared { get; set; }
}
