namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Bays;

public record struct BayId(string Airport, string Name)
{
    public override string ToString()
    {
        return $"{Airport}:{Name}";
    }
}