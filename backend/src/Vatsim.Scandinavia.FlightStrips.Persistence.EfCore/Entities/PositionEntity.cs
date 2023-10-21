namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

public class PositionEntity : IAirportTenant
{
    public int Id { get; set; }

    public required string Name { get; set; }

    public required string Frequency { get; set; }

    public string Airport { get; set; } = string.Empty;
}
