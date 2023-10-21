namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

public class OnlinePositionEntity : IAirportTenant, ISessionTenant
{
    public int Id { get; set; }

    public string Session { get; set; } = string.Empty;

    public string Airport { get; set; } = string.Empty;

    public required string PositionName { get; set; }

    public int PositionId { get; set; }

    public PositionEntity Position { get; set; } = null!;
}
