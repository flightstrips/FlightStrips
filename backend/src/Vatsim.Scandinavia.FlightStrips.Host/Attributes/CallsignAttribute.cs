using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Attributes;

[AttributeUsage(AttributeTargets.Property | AttributeTargets.Field | AttributeTargets.Parameter)]
public class CallsignAttribute : RegularExpressionAttribute
{
    private const string Regex = "^[a-ZA-Z0-9]*$";

    public CallsignAttribute() : base(Regex)
    {
    }
}
