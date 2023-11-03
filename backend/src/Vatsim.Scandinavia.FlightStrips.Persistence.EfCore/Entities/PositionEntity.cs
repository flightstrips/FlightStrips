using Microsoft.EntityFrameworkCore;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(Airport), nameof(Frequency))]
public class PositionEntity : IAirportTenant
{
    public required string Name { get; set; }

    public required string Frequency { get; set; }

    public string Airport { get; set; } = string.Empty;
}
