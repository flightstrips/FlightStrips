using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Entities;

public class Bay
{
    public Guid Id { get; set; }
    public required string Name { get; set; }
    public List<Strip> Strips { get; set; } = new List<Strip>();
}
