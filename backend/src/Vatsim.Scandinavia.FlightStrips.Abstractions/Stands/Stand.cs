using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Stands;

public record Stand(string Name, Location Location, int Radius)
{
    public int Radius { get; } =
        Radius <= 1000 ? Radius : throw new ArgumentException("Radius should be less than 1000");

    public bool IsWithin(Location position)
    {
        var distance = Location.Distance(position);

        return distance <= Radius;
    }
}
