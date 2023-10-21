using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

public class StripEntity : IAirportAndSessionTenant
{
    public int Id { get; set; }

    public string Session { get; set; } = string.Empty;
    public string Airport { get; set; } = string.Empty;
    public required string Callsign { get; set; }
    public string? Origin { get; set; }
    public string? Destination { get; set; }
    public int? Sequence { get; set; }
    public StripState State { get; set; }
    public bool Cleared { get; set; }

    /* TODO fix
    public string? Controller { get; set; }
    public string? NextController { get; set; }
    */

    public int BayId { get; set; }

    public virtual BayEntity Bay { get; set; } = null!;
}
