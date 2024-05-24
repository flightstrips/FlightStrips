using System.Globalization;

namespace Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;

public record struct Location(double Latitude, double Longitude)
{
    /// <summary>
    /// Parse from coordinate strings.
    /// </summary>
    public static Location FromCoordinateString(string latitude, string longitude)
    {
        var latSpan = latitude.AsSpan();
        var lngSpan = longitude.AsSpan();

        var north = latSpan[0] == 'N';
        var west = lngSpan[0] == 'W';

        var lat = double.Parse(latSpan[1..4], CultureInfo.InvariantCulture) +
                double.Parse(latSpan[5..7], CultureInfo.InvariantCulture) / 60 +
                double.Parse(latSpan[8..], CultureInfo.InvariantCulture) / 3600;

        var lng = double.Parse(lngSpan[1..4], CultureInfo.InvariantCulture) +
                double.Parse(lngSpan[5..7], CultureInfo.InvariantCulture) / 60 +
                double.Parse(lngSpan[8..], CultureInfo.InvariantCulture) / 3600;

        if (!north) lat *= -1;
        if (west) lng *= -1;

        return new Location(lat, lng);
    }
}

public static class LocationExtensions
{
    private const double EarthRadiusInMeters = 6376500.0;

    public static double Distance(this Location location, Location to)
    {
        var d1 = location.Latitude * (Math.PI / 180.0);
        var num1 = location.Longitude * (Math.PI / 180.0);
        var d2 = to.Latitude * (Math.PI / 180.0);
        var num2 = to.Longitude * (Math.PI / 180.0) - num1;
        var d3 = Math.Pow(Math.Sin((d2 - d1) / 2.0), 2.0) +
                 Math.Cos(d1) * Math.Cos(d2) * Math.Pow(Math.Sin(num2 / 2.0), 2.0);
        return EarthRadiusInMeters * (2.0 * Math.Atan2(Math.Sqrt(d3), Math.Sqrt(1.0 - d3)));
    }
}
